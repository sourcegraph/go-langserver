package main

import myhttp "net/http" // "net/http"

type unexpA struct {
	*myhttp.Client // net/http Client"
}

type unexpB struct {
	*unexpA
}

func main() {
	var x unexpB
	x.Transport.RoundTrip(nil) // "net/http Client Transport", "net/http RoundTripper RoundTrip"

	b := x
	c := b
	_ = c.unexpA.Transport.RoundTrip // "net/http Client Transport", "net/http RoundTripper RoundTrip"
}
