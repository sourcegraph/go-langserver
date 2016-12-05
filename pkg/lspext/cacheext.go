package lspext

import "encoding/json"

// CacheGetParams is the input for 'cache/get'. The response is a
// 'CacheItem'. This cache is global to a language server. IE it is shared
// amongst workspaces, but not amongst different language server types.
type CacheGetParams struct {
	Key string `json:"key"`
}

// CacheItem is the response for 'cache/get'.
type CacheItem struct {
	// Value is the value stored in the cache. It is a *json.RawMessage
	// since the type in the spec is `any`. It is nil if the value is not
	// in the cache.
	Value *json.RawMessage `json:"value,omitempty"`
}

// CacheSetParams is the input for 'cache/set'. It is a notify method, so
// does not have a response.
type CacheSetParams struct {
	Key string `json:"key"`
	// Value is the same as CacheItem.Value. Note when setting Value
	// cannot be nil.
	Value *json.RawMessage `json:"value"`
}
