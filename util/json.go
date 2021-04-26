package util

import "encoding/json"

func MarshalToString(i interface{}) string {
	if i == nil {
		return ""
	}
	data, err := json.Marshal(i)
	if err != nil {
		return ""
	}
	return string(data)
}

func MarshalToValue(obj interface{}) string {
	switch v := obj.(type) {
	case string:
		return v
	case *string:
		return *v
	default:
		return MarshalToString(v)
	}
}
