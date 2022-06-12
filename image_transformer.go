package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"sigs.k8s.io/kustomize/api/filters/imagetag"
	"sigs.k8s.io/kustomize/api/konfig/builtinpluginconsts"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filtersutil"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	fnConfigGroup      = "fn.kpt.dev"
	fnConfigVersion    = "v1alpha1"
	fnConfigAPIVersion = fnConfigGroup + "/" + fnConfigVersion
	fnConfigKind       = "SetImageFromConfigMap"
)

type SetImageInfo struct {
	// Name is a tag-less image name.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// NewName is the new name the image should align to
	NewName string `json:"newName,omitempty" yaml:"newName,omitempty"`

	// Registry is the registry the image should align to
	NewRegistry string `json:"newRegistry,omitempty" yaml:"newRegistry,omitempty"`

	// NewTag is the value used to replace the original tag.
	NewTag string `json:"newTag,omitempty" yaml:"newTag,omitempty"`
}

type SetImageConfigMap struct {
	// Name is the name of the config map the image transformation input is stored in
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

type SetImage struct {
	// ConfigMap that provides additional transform data input
	ConfigMap SetImageConfigMap `json:"configMap,omitempty" yaml:"configMap,omitempty"`
	// SetImageInfo is used to define the parameter used to transform the image
	//Image types.Image `json:"image,omitempty" yaml:"image,omitempty"`
	SetImageInfo SetImageInfo `json:"setImageInfo,omitempty" yaml:"setImageInfo,omitempty"`
	// AdditionalImageFields is used to specify additional fields to set image.
	AdditionalImageFields types.FsSlice `json:"additionalImageFields,omitempty" yaml:"additionalImageFields,omitempty"`
	// setImageResults is used internally to track which images were updated
	setImageResults setImageResults
}

// setImageResultKey is used as a unique identifier for set image results
type setImageResultKey struct {
	ResourceRef yaml.ResourceIdentifier
	// FilePath is the file path of the resource
	FilePath string
	// FileIndex is the file index of the resource
	FileIndex int
	// FieldPath is field path of the image field
	FieldPath string
}

// setImageResult maps a previous image value to a new image value where set-image is applied
// e.g. "nginx:1.20.2" -> "nginx:1.21.6"
type setImageResult struct {
	// CurrentValue is the value before applying the set-image mutation
	CurrentValue string
	// UpdatedValue is the value that will be set after applying set-image
	UpdatedValue string
}

// setImageResults tracks the number of images updated matching the key
type setImageResults map[setImageResultKey][]setImageResult

// getDefaultImageFields returns default image FieldSpecs
func getDefaultImageFields() (types.FsSlice, error) {
	type defaultConfig struct {
		FieldSpecs types.FsSlice `json:"images,omitempty" yaml:"images,omitempty"`
	}
	defaultConfigString := builtinpluginconsts.GetDefaultFieldSpecsAsMap()["images"]
	var tc defaultConfig
	err := yaml.Unmarshal([]byte(defaultConfigString), &tc)
	return tc.FieldSpecs, err
}

// Config initializes SetImage from a functionConfig fn.KubeObject
func (si *SetImage) Config(rl *fn.ResourceList) (error, bool) {
	si.ConfigMap = SetImageConfigMap{}
	//fmt.Printf("functionConfig: %v\n", rl.FunctionConfig)
	switch {
	case rl.FunctionConfig.IsGVK("", "v1", "ConfigMap"):
		rl.FunctionConfig.GetOrDie(&si.ConfigMap, "data")
	case rl.FunctionConfig.IsGVK(fnConfigGroup, fnConfigVersion, fnConfigKind):
		rl.FunctionConfig.AsOrDie(si)
	default:
		return fmt.Errorf("`functionConfig` must be a `ConfigMap` or `%s`", fnConfigKind), false
	}

	if err := si.getSetImageConfig(rl.Items); err != nil {
		return err, false
	}
	defaultImageFields, err := getDefaultImageFields()
	if err != nil {
		return err, false
	}
	si.AdditionalImageFields = append(si.AdditionalImageFields, defaultImageFields...)

	return nil, true
}

// getSetImageConfig validates and retrieves the input passed into via the functionConfig
func (si *SetImage) getSetImageConfig(items fn.KubeObjects) error {
	for _, kubeObject := range items {
		if kubeObject.IsGVK("", "v1", "ConfigMap") {
			if si.ConfigMap.Name == kubeObject.GetName() {
				//si.Image = types.Image{}
				//kubeObject.GetOrDie(&si.Image, "data")
				si.SetImageInfo = SetImageInfo{}
				kubeObject.GetOrDie(&si.SetImageInfo, "data")
			}
		}
	}
	return nil
}

func (si *SetImage) Transform(rl *fn.ResourceList) error {
	// only transform the items if we have a valid key
	if si.SetImageInfo.Name != "" {
		//return fmt.Errorf("`we have data: %v`", si.SetImageInfo)
		var transformedItems []*fn.KubeObject
		si.setImageResults = make(setImageResults)
		for _, obj := range rl.Items {
			objRN, err := yaml.Parse(obj.String())
			if err != nil {
				return err
			}
			filter := imagetag.Filter{
				//ImageTag: si.Image,
				ImageTag: types.Image{
					Name:    si.SetImageInfo.Name,
					NewName: si.SetImageInfo.NewName,
					NewTag:  si.SetImageInfo.NewTag,
				},
				FsSlice: si.AdditionalImageFields,
			}
			//return fmt.Errorf("`we have data: %s %s %s with %v`", obj.GetName(), obj.GetKind(), obj.GetAPIVersion(), filter)

			filter.WithMutationTracker(si.mutationTracker(objRN, obj))
			err = filtersutil.ApplyToJSON(filter, objRN)
			if err != nil {
				return err
			}
			obj, err = fn.ParseKubeObject([]byte(objRN.MustString()))
			if err != nil {
				return err
			}
			transformedItems = append(transformedItems, obj)
		}
		rl.Items = transformedItems
	}

	return nil
}

func (si *SetImage) mutationTracker(objRN *yaml.RNode, ko *fn.KubeObject) func(key, value, tag string, node *yaml.RNode) {
	filePath, fileIndexStr, _ := kioutil.GetFileAnnotations(objRN)
	fileIndex, _ := strconv.Atoi(fileIndexStr)
	return func(key, value, tag string, node *yaml.RNode) {
		currentValue := node.YNode().Value
		rk := setImageResultKey{
			ResourceRef: yaml.ResourceIdentifier{
				TypeMeta: yaml.TypeMeta{
					APIVersion: ko.GetAPIVersion(),
					Kind:       ko.GetKind(),
				},
				NameMeta: yaml.NameMeta{
					Name:      ko.GetName(),
					Namespace: ko.GetNamespace(),
				},
			},
			FilePath:  filePath,
			FileIndex: fileIndex,
			FieldPath: strings.Join(node.FieldPath(), "."),
		}
		si.setImageResults[rk] = append(si.setImageResults[rk], setImageResult{
			CurrentValue: currentValue,
			UpdatedValue: value,
		})
	}
}

// SdkResults returns fn.Results representing which images were updated
func (si *SetImage) SdkResults() fn.Results {
	results := fn.Results{}
	if len(si.setImageResults) == 0 {
		results = append(results, &fn.Result{
			Message:  "no images changed",
			Severity: fn.Info,
		})
		return results
	}
	for k, v := range si.setImageResults {
		resourceRef := k.ResourceRef
		for _, sir := range v {
			results = append(results, &fn.Result{
				Message: fmt.Sprintf("set image from %s to %s", sir.CurrentValue, sir.UpdatedValue),
				Field: &fn.Field{
					Path:          k.FieldPath,
					CurrentValue:  sir.CurrentValue,
					ProposedValue: sir.UpdatedValue,
				},
				File:        &fn.File{Path: k.FilePath, Index: k.FileIndex},
				Severity:    fn.Info,
				ResourceRef: &resourceRef,
			})
		}
	}
	results.Sort()
	return results
}
