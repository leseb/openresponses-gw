// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extproc

import (
	"encoding/json"
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

func makeHeader(key, value string) *corev3.HeaderValueOption {
	return &corev3.HeaderValueOption{
		Header: &corev3.HeaderValue{
			Key:      key,
			RawValue: []byte(value),
		},
	}
}

func immediateResponseMsg(statusCode int, headers map[string]string, body []byte) *extprocv3.ProcessingResponse {
	hdrs := make([]*corev3.HeaderValueOption, 0, len(headers))
	for k, v := range headers {
		hdrs = append(hdrs, makeHeader(k, v))
	}
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocv3.ImmediateResponse{
				Status: &typev3.HttpStatus{
					Code: typev3.StatusCode(statusCode),
				},
				Headers: &extprocv3.HeaderMutation{
					SetHeaders: hdrs,
				},
				Body: body,
			},
		},
	}
}

func streamHeadersMsg(statusCode int, headers map[string]string) *extprocv3.ProcessingResponse {
	hdrs := make([]*corev3.HeaderValue, 0, len(headers)+1)
	hdrs = append(hdrs, &corev3.HeaderValue{
		Key:      ":status",
		RawValue: []byte(fmt.Sprintf("%d", statusCode)),
	})
	for k, v := range headers {
		hdrs = append(hdrs, &corev3.HeaderValue{
			Key:      k,
			RawValue: []byte(v),
		})
	}
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_StreamedImmediateResponse{
			StreamedImmediateResponse: &extprocv3.StreamedImmediateResponse{
				Response: &extprocv3.StreamedImmediateResponse_HeadersResponse{
					HeadersResponse: &extprocv3.HttpHeaders{
						Headers: &corev3.HeaderMap{
							Headers: hdrs,
						},
					},
				},
			},
		},
	}
}

func streamBodyMsg(data []byte, endOfStream bool) *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_StreamedImmediateResponse{
			StreamedImmediateResponse: &extprocv3.StreamedImmediateResponse{
				Response: &extprocv3.StreamedImmediateResponse_BodyResponse{
					BodyResponse: &extprocv3.StreamedBodyResponse{
						Body:        data,
						EndOfStream: endOfStream,
					},
				},
			},
		},
	}
}

func errorResponse(statusCode int, errType, message string) *extprocv3.ProcessingResponse {
	body, _ := json.Marshal(map[string]interface{}{
		"error": map[string]string{
			"type":    errType,
			"message": message,
		},
	})
	return immediateResponseMsg(statusCode, map[string]string{
		"content-type": "application/json",
	}, body)
}

func passthroughResponse() *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{},
		},
	}
}
