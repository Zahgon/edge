package edge

import (
	"github.com/edge-js/edge-go/internal/edgert"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/js"
)

// superChargedComponents lets components be used as tags: every component of
// the mounted disks becomes a tag named after its path ("form/input-text"
// becomes @form.inputText, "admin::panel" @admin.panel), which the compilers
// claim and the "tag" processor turns into @component.
type superChargedComponents struct {
	edge *Edge
	// components maps tag names to component names.
	components map[string]string
}

// superCharged is the plugin's state. Like the original's module variable it
// is shared by every Edge instance: the first run of the plugin on an
// instance replaces it, and later runs on any instance refresh it.
var superCharged *superChargedComponents

// pluginSuperCharged is the bundled plugin: it sets the components up on its
// first run and refreshes the list of components on every run.
func pluginSuperCharged(e *Edge, firstRun bool, _ any) error {
	if firstRun {
		superCharged = newSuperChargedComponents(e)
	}
	return superCharged.refreshComponents()
}

func newSuperChargedComponents(e *Edge) *superChargedComponents {
	s := &superChargedComponents{edge: e, components: map[string]string{}}
	s.claimTags()
	s.transformTags()
	return s
}

// refreshComponents lists the components of the loader of the instance the
// plugin was set up with.
func (s *superChargedComponents) refreshComponents() error {
	var disks []DiskComponents
	if lister, ok := s.edge.loader.(componentLister); ok {
		list, err := lister.listComponents()
		if err != nil {
			return err
		}
		disks = list
	} else {
		disks = s.edge.loader.ListComponents()
	}
	components := map[string]string{}
	for _, disk := range disks {
		for _, component := range disk.Components {
			components[component.TagName] = component.ComponentName
		}
	}
	s.components = components
	return nil
}

// component returns the component a tag name stands for. The components are
// a plain object in the original, so a name it inherits from
// Object.prototype stands for the text of the inherited value.
func (s *superChargedComponents) component(name string) (string, bool) {
	if component, ok := s.components[name]; ok {
		return component, component != ""
	}
	inherited, err := edgert.InheritedProperty(name)
	if err != nil {
		// Object.prototype's own properties are data properties and the
		// __proto__ accessor, whose getter returns the object's prototype.
		panic(err)
	}
	if !js.ToBoolean(inherited) {
		return "", false
	}
	return js.ToString(inherited), true
}

// claimTags makes both compilers read component tags as seekable blocks.
func (s *superChargedComponents) claimTags() {
	claim := func(name string) *Tag {
		if _, ok := s.component(name); ok {
			return &Tag{Seekable: true, Block: true}
		}
		return nil
	}
	s.edge.compiler.ClaimTag(claim)
	s.edge.asyncCompiler.ClaimTag(claim)
}

// transformTags registers the "tag" processor that turns a component tag
// into @component, with the component name as its first argument.
func (s *superChargedComponents) transformTags() {
	s.edge.processor.Process("tag", func(data ProcessorData) (string, bool, error) {
		tag := data.Tag
		component, ok := s.component(tag.Properties.Name)
		if !ok {
			return "", false, nil
		}
		tag.Properties.Name = "component"
		if jsstr.TrimSpace(tag.Properties.JsArg) == "" {
			tag.Properties.JsArg = "'" + component + "'"
		} else {
			tag.Properties.JsArg = "'" + component + "'," + tag.Properties.JsArg
		}
		return "", false, nil
	})
}
