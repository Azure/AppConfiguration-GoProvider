// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package afd

import (
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type AnonymousRequestPipelinePolicy struct{}

type RemoveSyncTokenPipelinePolicy struct{}

func NewAnonymousRequestPipelinePolicy() *AnonymousRequestPipelinePolicy {
	return &AnonymousRequestPipelinePolicy{}
}

func NewRemoveSyncTokenPipelinePolicy() *RemoveSyncTokenPipelinePolicy {
	return &RemoveSyncTokenPipelinePolicy{}
}

func (p *AnonymousRequestPipelinePolicy) Do(req *policy.Request) (*http.Response, error) {
	if req.Raw().Header.Get("Authorization") != "" {
		req.Raw().Header.Del("Authorization")
	}

	return req.Next()
}

func (p *RemoveSyncTokenPipelinePolicy) Do(req *policy.Request) (*http.Response, error) {
	if req.Raw().Header.Get("Sync-Token") != "" {
		req.Raw().Header.Del("Sync-Token")
	}

	return req.Next()
}
