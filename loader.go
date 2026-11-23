package edge

import (
	"errors"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/edge-js/edge-go/internal/edgert"
	"github.com/edge-js/edge-go/internal/nodefs"
	"github.com/edge-js/edge-go/internal/nodepath"
	"github.com/edge-js/edge-go/internal/strutil"
	"github.com/edge-js/edge-go/js"
)

// LoaderTemplate is what a loader resolves a template to: { template }. A
// template registered with Register must be a string.
type LoaderTemplate struct{ Template any }

// ComponentInfo is one component of a disk: the path the @component tag takes
// and the tag name the supercharged plugin gives it.
type ComponentInfo struct{ ComponentName, TagName string }

// DiskComponents lists the components of one mounted disk.
type DiskComponents struct {
	DiskName   string
	Components []ComponentInfo
}

// DiskTemplates lists the templates of one mounted disk.
type DiskTemplates struct {
	DiskName  string
	Templates []string
}

// LoaderContract is the interface Edge loads templates through. Custom loaders
// implement it; methods that throw in the original return an error.
type LoaderContract interface {
	Mounted() *js.Object   // loader.mounted: disk name -> directory
	Templates() *js.Object // loader.templates: path -> { template }
	Mount(diskName, dirPath string)
	Unmount(diskName string)
	Resolve(templatePath string) (LoaderTemplate, error)
	MakePath(templatePath string) (string, error)
	Register(templatePath string, contents LoaderTemplate) error
	Remove(templatePath string)
	ListComponents() []DiskComponents
}

// componentLister is implemented by loaders whose component listing can fail,
// as the default loader's directory reads can. The supercharged plugin uses it
// to report the failure as an error rather than a panic.
type componentLister interface {
	listComponents() ([]DiskComponents, error)
}

// Loader is the default loader: it reads templates from the mounted
// directories synchronously and keeps templates registered in memory.
type Loader struct {
	mountedDirs   orderedMap[string]
	preRegistered orderedMap[LoaderTemplate]
}

// NewLoader returns a loader with no disk mounted.
func NewLoader() *Loader {
	return &Loader{}
}

// componentsDirName is the directory of every disk that holds its components.
const componentsDirName = "components"

// Mounted returns an object of the mounted directories keyed by disk name, in
// the order they were first mounted.
func (l *Loader) Mounted() *js.Object {
	o := js.NewObject()
	for _, k := range l.mountedDirs.keys {
		o.Set(k, l.mountedDirs.values[k])
	}
	return o
}

// Templates returns an object of the templates registered in memory, keyed by
// their path.
func (l *Loader) Templates() *js.Object {
	o := js.NewObject()
	for _, k := range l.preRegistered.keys {
		o.Set(k, js.ObjectOf("template", l.preRegistered.values[k].Template))
	}
	return o
}

// Mount mounts a directory under a disk name. Templates of the "default" disk
// resolve without the disk prefix.
func (l *Loader) Mount(diskName, dirPath string) {
	l.mountedDirs.set(diskName, dirPath)
}

// Unmount removes a mounted disk.
func (l *Loader) Unmount(diskName string) {
	l.mountedDirs.remove(diskName)
}

// extractDiskAndTemplateName splits "disk::path" into the disk name ("default"
// without a prefix) and the template file name, adding the ".edge" extension
// the way the original does: the part after the first ".edge" replaces the
// extension, so "a.edgex" names "a.x".
func extractDiskAndTemplateName(templatePath string) (string, string) {
	parts := strings.Split(templatePath, "::")
	disk, rest := parts[0], parts[1:]
	if len(rest) == 0 {
		rest = []string{disk}
		disk = "default"
	}
	pieces := strings.Split(strings.Join(rest, "::"), ".edge")
	ext := "edge"
	if len(pieces) > 1 && pieces[1] != "" {
		ext = pieces[1]
	}
	return disk, pieces[0] + "." + ext
}

// MakePath returns the path of a template: a registered template's own path,
// an absolute path as it is, or the template joined to its disk's directory.
// An unmounted disk is an error.
func (l *Loader) MakePath(templatePath string) (string, error) {
	if l.preRegistered.has(templatePath) {
		return templatePath, nil
	}
	if nodepath.IsAbsolute(templatePath) {
		return templatePath, nil
	}
	diskName, template := extractDiskAndTemplateName(templatePath)
	mountedDir := l.mountedDirs.values[diskName]
	if mountedDir == "" {
		return "", js.NewError(`"` + diskName + `" namespace is not mounted`)
	}
	return nodepath.Join(mountedDir, template), nil
}

// Resolve returns a registered template, or reads the template from the disk.
func (l *Loader) Resolve(templatePath string) (LoaderTemplate, error) {
	if t, ok := l.preRegistered.get(templatePath); ok {
		return t, nil
	}
	if !nodepath.IsAbsolute(templatePath) {
		p, err := l.MakePath(templatePath)
		if err != nil {
			return LoaderTemplate{}, err
		}
		templatePath = p
	}
	contents, err := readTemplateContents(templatePath)
	if err != nil {
		return LoaderTemplate{}, err
	}
	return LoaderTemplate{Template: contents}, nil
}

// Register registers an in-memory template. Its contents must be a string and
// a path cannot be registered twice.
func (l *Loader) Register(templatePath string, contents LoaderTemplate) error {
	if _, ok := contents.Template.(string); !ok {
		return js.NewError("Make sure to define the template content as a string")
	}
	if l.preRegistered.has(templatePath) {
		return js.NewError(`Cannot override previously registered "` + templatePath + `" template`)
	}
	l.preRegistered.set(templatePath, contents)
	return nil
}

// Remove removes a registered template.
func (l *Loader) Remove(templatePath string) {
	l.preRegistered.remove(templatePath)
}

// ListComponents returns the components of every mounted disk: the in-memory
// templates (on the default disk) and the ".edge" files under each disk's
// components directory. The method has no error result, so a components
// directory that cannot be read, which the original reports by throwing,
// panics with the file-system error.
func (l *Loader) ListComponents() []DiskComponents {
	list, err := l.listComponents()
	if err != nil {
		panic(err)
	}
	return list
}

func (l *Loader) listComponents() ([]DiskComponents, error) {
	out := make([]DiskComponents, 0, len(l.mountedDirs.keys))
	for _, diskName := range l.mountedDirs.keys {
		components, err := l.diskComponents(diskName)
		if err != nil {
			return nil, err
		}
		out = append(out, DiskComponents{DiskName: diskName, Components: components})
	}
	return out, nil
}

// ListTemplates returns the templates of every mounted disk: the in-memory
// templates (on the default disk) and every ".edge" file under the disk's
// directory, without the extension. Like ListComponents, it panics with the
// file-system error of a directory that cannot be read.
func (l *Loader) ListTemplates() []DiskTemplates {
	out := make([]DiskTemplates, 0, len(l.mountedDirs.keys))
	for _, diskName := range l.mountedDirs.keys {
		templates, err := l.diskTemplates(diskName)
		if err != nil {
			panic(err)
		}
		out = append(out, DiskTemplates{DiskName: diskName, Templates: templates})
	}
	return out
}

// diskComponents lists the components of one disk.
func (l *Loader) diskComponents(diskName string) ([]ComponentInfo, error) {
	type file struct{ fileName, componentPath string }
	var files []file
	if diskName == "default" {
		for _, template := range l.preRegistered.keys {
			files = append(files, file{template, template})
		}
	}
	componentsPath := nodepath.Join(l.mountedDirs.values[diskName], componentsDirName)
	if existsSync(componentsPath) {
		entries, err := readdirRecursive(componentsPath)
		if err != nil {
			return nil, err
		}
		cwd, err := processCwd()
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.isFile || !strings.HasSuffix(entry.name, ".edge") {
				continue
			}
			rel := nodepath.Relative(cwd, componentsPath, nodepath.Join(entry.parentPath, entry.name))
			fileName := strings.TrimSuffix(strutil.ToUnixSlash(rel), ".edge")
			files = append(files, file{fileName, componentsDirName + "/" + fileName})
		}
	}
	components := make([]ComponentInfo, 0, len(files))
	for _, f := range files {
		var segments []string
		for i, segment := range strings.Split(f.fileName, "/") {
			if i == 0 || segment != "index" {
				segments = append(segments, strutil.CamelCase(segment))
			}
		}
		tagName := strings.Join(segments, ".")
		info := ComponentInfo{ComponentName: f.componentPath, TagName: tagName}
		if diskName != "default" {
			info = ComponentInfo{ComponentName: diskName + "::" + f.componentPath, TagName: diskName + "." + tagName}
		}
		components = append(components, info)
	}
	return components, nil
}

// diskTemplates lists the templates of one disk.
func (l *Loader) diskTemplates(diskName string) ([]string, error) {
	diskBasePath := l.mountedDirs.values[diskName]
	var files []string
	if diskName == "default" {
		files = append(files, l.preRegistered.keys...)
	}
	if existsSync(diskBasePath) {
		entries, err := readdirRecursive(diskBasePath)
		if err != nil {
			return nil, err
		}
		cwd, err := processCwd()
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.isFile && strings.HasSuffix(entry.name, ".edge") {
				rel := nodepath.Relative(cwd, diskBasePath, nodepath.Join(entry.parentPath, entry.name))
				files = append(files, strings.TrimSuffix(rel, ".edge"))
			}
		}
	}
	templates := make([]string, len(files))
	for i, f := range files {
		name := strings.TrimSuffix(strutil.ToUnixSlash(f), ".edge")
		if diskName != "default" {
			name = diskName + "::" + name
		}
		templates[i] = name
	}
	return templates, nil
}

// processCwd is process.cwd(), which path.relative resolves relative paths
// against.
func processCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nodefs.NewError(err, "uv_cwd", "").JSError()
	}
	return cwd, nil
}

// existsSync is fs.existsSync: whether stat succeeds on the path.
func existsSync(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// dirent is an entry fs.readdirSync lists with withFileTypes.
type dirent struct {
	name, parentPath string
	isFile           bool
}

// readdirRecursive is fs.readdirSync(dir, { recursive: true, withFileTypes:
// true }): each directory is read in scandir's (sorted) order, and the
// directories found are read after the current level, breadth first.
// Symbolic links are listed without being followed.
func readdirRecursive(base string) ([]dirent, error) {
	var out []dirent
	queue := []string{base}
	for i := 0; i < len(queue); i++ {
		dir := queue[i]
		names, err := nodefs.Readdir(dir)
		if err != nil {
			return nil, err.(*nodefs.Error).JSError()
		}
		for _, name := range names {
			// The type comes from lstat, as a dirent's does when scandir
			// reports none; an entry removed in between has no type.
			var mode os.FileMode = os.ModeIrregular
			if fi, err := os.Lstat(nodepath.Join(dir, name)); err == nil {
				mode = fi.Mode()
			}
			out = append(out, dirent{name: name, parentPath: dir, isFile: mode.IsRegular()})
			if mode.IsDir() {
				queue = append(queue, nodepath.Join(dir, name))
			}
		}
	}
	return out, nil
}

// invalidPathType is the TypeError path.isAbsolute throws for a template path
// that is not a string, as Node's validateString reports it.
func invalidPathType(value any) error {
	received, err := describeReceived(value)
	if err != nil {
		return err
	}
	e := js.NewTypeError(`The "path" argument must be of type string. Received ` + received)
	e.Props = js.ObjectOf("code", "ERR_INVALID_ARG_TYPE")
	return e
}

// describeReceived describes a value that is not a string the way Node's
// ERR_INVALID_ARG_TYPE messages do.
func describeReceived(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "null", nil
	case js.UndefinedType:
		return "undefined", nil
	case float64:
		if v == 0 && math.Signbit(v) {
			return "type number (-0)", nil
		}
		return "type number (" + js.NumberToString(v) + ")", nil
	case bool:
		return "type boolean (" + strconv.FormatBool(v) + ")", nil
	case *js.Function:
		name, err := edgert.Realm().Get(v, "name")
		if err != nil {
			return "", err
		}
		text, err := edgert.Realm().ToString(name)
		return "function " + text, err
	}
	ctor, err := edgert.Realm().Get(value, "constructor")
	if err != nil {
		return "", err
	}
	if js.ToBoolean(ctor) {
		hasName, err := edgert.Realm().Has(ctor, "name")
		if err != nil {
			return "", err
		}
		if hasName {
			name, err := edgert.Realm().Get(ctor, "name")
			if err != nil {
				return "", err
			}
			text, err := edgert.Realm().ToString(name)
			return "an instance of " + text, err
		}
	}
	return edgert.Realm().Inspect(value), nil
}

// readTemplateContents is readFileSync(absPath, "utf-8") with the loader's
// message for a missing file.
func readTemplateContents(absPath string) (string, error) {
	f, err := os.Open(absPath)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return "", js.NewError(`Cannot resolve "` + absPath + `". Make sure the file exists`)
		}
		return "", nodefs.NewError(err, "open", absPath).JSError()
	}
	data, err := io.ReadAll(f)
	if cerr := f.Close(); err == nil && cerr != nil {
		return "", nodefs.NewError(cerr, "close", absPath).JSError()
	}
	if err != nil {
		return "", readError(err)
	}
	return js.DecodeUTF8(data), nil
}

// readError is the error readFileSync throws when reading an open file fails
// (EISDIR for a directory): Node reports the read syscall without a path.
func readError(err error) *js.Error {
	e := nodefs.NewError(err, "read", "")
	e.Message = strings.TrimSuffix(e.Error(), " ''")
	jsErr := e.JSError()
	jsErr.Props.Delete("path")
	return jsErr
}

// orderedMap is a string-keyed map that remembers insertion order, as a
// JavaScript Map does: setting an existing key keeps its position.
type orderedMap[V any] struct {
	keys   []string
	values map[string]V
}

func (m *orderedMap[V]) has(k string) bool {
	_, ok := m.values[k]
	return ok
}

func (m *orderedMap[V]) get(k string) (V, bool) {
	v, ok := m.values[k]
	return v, ok
}

func (m *orderedMap[V]) set(k string, v V) {
	if m.values == nil {
		m.values = map[string]V{}
	}
	if _, ok := m.values[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.values[k] = v
}

func (m *orderedMap[V]) remove(k string) {
	if _, ok := m.values[k]; !ok {
		return
	}
	delete(m.values, k)
	for i, key := range m.keys {
		if key == k {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			return
		}
	}
}
