// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package afd

import (
	"context"
	"math"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type EmptyTokenCredential struct{}

func (e *EmptyTokenCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{
		Token:     "",
		ExpiresOn: time.Unix(math.MaxInt64, 0),
	}, nil
}

func NewEmptyTokenCredential() azcore.TokenCredential {
	return &EmptyTokenCredential{}
}
