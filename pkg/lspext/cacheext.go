package lspext

// CacheGetParams is the input for 'cache/xget'. The response is a
// 'CacheItem'. This cache is global to a language server. IE it is shared
// amongst workspaces, but not amongst different language server types.
type CacheGetParams struct {
	Key string `json:"key"`
}

// CacheItem is the response for 'cache/xget'.
type CacheItem struct {
	// Ok is true if the item was successfuly retrieved from the cache.
	Ok bool `json:"ok"`
	// Value is the value stored in the cache. It is present if Ok is true.
	Value string `json:"value,omitempty"`
}

// CacheSetParams is the input for 'cache/xset'. It is a notify method, so
// does not have a response.
type CacheSetParams struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
