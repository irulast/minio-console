// Copyright (c) 2026 PGSTY
//
// This file is part of the Silo object storage console.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-openapi/runtime"
	"github.com/minio/console/api/operations"
	bucketAPI "github.com/minio/console/api/operations/bucket"
	"github.com/minio/console/models"
	"github.com/stretchr/testify/require"
)

func TestBucketQuotaResponses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		quota      string
		want       string
		nilDetails bool
	}{
		{
			name:  "modern_size",
			quota: `{"size":118111600640,"quotatype":"hard"}`,
			want:  `{"quota":118111600640,"type":"hard"}`,
		},
		{
			name:  "legacy_quota",
			quota: `{"quota":107374182400,"quotatype":"hard"}`,
			want:  `{"quota":107374182400,"type":"hard"}`,
		},
		{
			name:  "modern_size_over_legacy_quota",
			quota: `{"size":118111600640,"quota":107374182400,"quotatype":"hard"}`,
			want:  `{"quota":118111600640,"type":"hard"}`,
		},
		{
			name:  "modern_size_below_legacy_quota",
			quota: `{"size":107374182400,"quota":118111600640,"quotatype":"hard"}`,
			want:  `{"quota":107374182400,"type":"hard"}`,
		},
		{
			name:  "empty_quota",
			quota: `{}`,
			want:  `{}`,
		},
		{
			name:  "zero_hard_quota",
			quota: `{"size":0,"quota":0,"quotatype":"hard"}`,
			want:  `{"type":"hard"}`,
		},
		{
			name:  "nil_quota",
			quota: `null`,
			want:  `{}`,
		},
		{
			name:       "nil_details",
			quota:      `null`,
			want:       `{}`,
			nilDetails: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			details := `{"quota":` + tc.quota + `}`
			if tc.nilDetails {
				details = `null`
			}
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected admin request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case accountInfoAdminPath:
					_, _ = fmt.Fprintf(w, `{"Buckets":[{"name":"quota-bucket","details":%s}]}`, details)
				case "/minio/admin/v3/datausageinfo":
					_, _ = fmt.Fprint(w, `{}`)
				case "/minio/admin/v3/get-bucket-quota":
					if r.URL.Query().Get("bucket") != "quota-bucket" {
						t.Errorf("unexpected bucket quota request: %s", r.URL)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					_, _ = fmt.Fprint(w, tc.quota)
				default:
					t.Errorf("unexpected admin request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			t.Cleanup(backend.Close)
			t.Setenv(ConsoleMinIOServer, backend.URL)
			api := &operations.ConsoleAPI{}
			registerBucketsHandlers(api)
			registerBucketQuotaHandlers(api)
			session := &models.Principal{STSAccessKeyID: "test-access", STSSecretAccessKey: "test-secret"}

			t.Run("bucket_list", func(t *testing.T) {
				params := bucketAPI.ListBucketsParams{
					HTTPRequest: httptest.NewRequest(http.MethodGet, "/api/v1/buckets", nil),
				}
				response := api.BucketListBucketsHandler.Handle(params, session)
				rec := httptest.NewRecorder()
				response.WriteResponse(rec, runtime.JSONProducer())
				t.Logf("bucket list JSON: %s", rec.Body.String())
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				var list struct {
					Buckets []struct {
						Name    string `json:"name"`
						Details struct {
							Quota json.RawMessage `json:"quota"`
						} `json:"details"`
					} `json:"buckets"`
					Total int64 `json:"total"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
				require.Equal(t, int64(1), list.Total)
				require.Len(t, list.Buckets, 1)
				require.Equal(t, "quota-bucket", list.Buckets[0].Name)
				if tc.quota == "null" || tc.nilDetails {
					require.Nil(t, list.Buckets[0].Details.Quota)
				} else {
					require.JSONEq(t, tc.want, string(list.Buckets[0].Details.Quota))
				}
			})

			t.Run("bucket_quota", func(t *testing.T) {
				params := bucketAPI.GetBucketQuotaParams{
					HTTPRequest: httptest.NewRequest(http.MethodGet, "/api/v1/buckets/quota-bucket/quota", nil),
					Name:        "quota-bucket",
				}
				response := api.BucketGetBucketQuotaHandler.Handle(params, session)
				rec := httptest.NewRecorder()
				response.WriteResponse(rec, runtime.JSONProducer())
				t.Logf("bucket quota JSON: %s", rec.Body.String())
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.JSONEq(t, tc.want, rec.Body.String())
			})
		})
	}
}
