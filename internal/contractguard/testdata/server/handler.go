// Fixture server: writes live_field and kind_tag, never dead_field.
package server

type thingResponse struct {
	LiveField int64
	DeadField int64
	KindTag   int32
}

func build() thingResponse {
	resp := thingResponse{LiveField: 42}
	resp.KindTag = 1
	return resp
}
