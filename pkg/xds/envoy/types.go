package envoy

import "sort"

type ClusterInfo struct {
	Name   string
	Weight uint32
	Tags   Tags
}

type Tags map[string]string

func (t Tags) WithoutTag(tag string) Tags {
	result := Tags{}
	for tagName, tagValue := range t {
		if tag != tagName {
			result[tagName] = tagValue
		}
	}
	return result
}

func (t Tags) Keys() []string {
	var keys []string
	for key := range t {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type Clusters map[string][]ClusterInfo

func (c Clusters) ClusterNames() []string {
	var keys []string
	for key := range c {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (c Clusters) Add(infos ...ClusterInfo) {
	for _, info := range infos {
		c[info.Name] = append(c[info.Name], info)
	}
}

func (c Clusters) Tags(name string) []Tags {
	var result []Tags
	for _, info := range c[name] {
		result = append(result, info.Tags)
	}
	return result
}
