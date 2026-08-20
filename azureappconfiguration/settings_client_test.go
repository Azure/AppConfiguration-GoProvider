// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type featureFlagPagerTestClient struct {
	appConfigClient
	selector azappconfig.FeatureFlagSelector
	options  *azappconfig.ListFeatureFlagsOptions
	pages    []azappconfig.ListFeatureFlagsPageResponse
	err      error
}

func (c *featureFlagPagerTestClient) NewListFeatureFlagsPager(selector azappconfig.FeatureFlagSelector, options *azappconfig.ListFeatureFlagsOptions) *runtime.Pager[azappconfig.ListFeatureFlagsPageResponse] {
	c.selector = selector
	c.options = options

	pageIndex := 0
	return runtime.NewPager(runtime.PagingHandler[azappconfig.ListFeatureFlagsPageResponse]{
		More: func(azappconfig.ListFeatureFlagsPageResponse) bool {
			return pageIndex < len(c.pages)
		},
		Fetcher: func(context.Context, *azappconfig.ListFeatureFlagsPageResponse) (azappconfig.ListFeatureFlagsPageResponse, error) {
			if c.err != nil {
				return azappconfig.ListFeatureFlagsPageResponse{}, c.err
			}

			page := c.pages[pageIndex]
			pageIndex++
			return page, nil
		},
	})
}

func TestEnhFFETagsClientCheckIfETagChanged(t *testing.T) {
	firstETag := azcore.ETag(`"first"`)
	secondETag := azcore.ETag(`"second"`)
	changedETag := azcore.ETag(`"changed"`)
	pagerErr := errors.New("list feature flags")

	tests := []struct {
		name        string
		pages       []azappconfig.ListFeatureFlagsPageResponse
		err         error
		wantChanged bool
		wantErr     error
	}{
		{
			name: "unchanged pages",
			pages: []azappconfig.ListFeatureFlagsPageResponse{
				{ETag: nil},
				{ETag: nil},
			},
		},
		{
			name: "changed page",
			pages: []azappconfig.ListFeatureFlagsPageResponse{
				{ETag: nil},
				{ETag: &changedETag},
			},
			wantChanged: true,
		},
		{
			name: "page added",
			pages: []azappconfig.ListFeatureFlagsPageResponse{
				{ETag: nil},
				{ETag: nil},
				{ETag: nil},
			},
			wantChanged: true,
		},
		{
			name: "page removed",
			pages: []azappconfig.ListFeatureFlagsPageResponse{
				{ETag: nil},
			},
			wantChanged: true,
		},
		{
			name:    "pager error",
			pages:   []azappconfig.ListFeatureFlagsPageResponse{{}},
			err:     pagerErr,
			wantErr: pagerErr,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &featureFlagPagerTestClient{
				pages: test.pages,
				err:   test.err,
			}
			selector := comparableSelector{
				KeyFilter:   "Beta*",
				LabelFilter: "production",
				TagFilters:  `["region=west"]`,
			}
			monitor := &enhFFETagsClient{
				pageETags: map[comparableSelector][]*azcore.ETag{
					selector: {&firstETag, &secondETag},
				},
				client: client,
			}

			changed, err := monitor.checkIfETagChanged(context.Background())

			assert.Equal(t, test.wantChanged, changed)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}

			require.NotNil(t, client.options)
			require.Len(t, client.options.MatchConditions, 2)
			assert.Equal(t, &firstETag, client.options.MatchConditions[0].IfNoneMatch)
			assert.Equal(t, &secondETag, client.options.MatchConditions[1].IfNoneMatch)
			assert.Equal(t, "Beta*", *client.selector.NameFilter)
			assert.Equal(t, "production", *client.selector.LabelFilter)
			assert.Equal(t, []string{"region=west"}, client.selector.TagsFilter)
		})
	}
}
