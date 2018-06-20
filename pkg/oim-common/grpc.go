/*
Copyright (C) 2018 Intel Corporation.

SPDX-License-Identifier: Apache-2.0
*/

package oimcommon

import (
	"context"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
)

// GRPCDialer can be used with grpc.WithDialer. It supports
// addresses of the format defined for ParseEndpoint.
// Necessary because of https://github.com/grpc/grpc-go/issues/1741.
func GRPCDialer(endpoint string, t time.Duration) (net.Conn, error) {
	network, address, err := ParseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(t))
	defer cancel()
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

// ChooseDialOpts sets certain default options for the given endpoint,
// then adds the ones given as additional parameters. For unix://
// endpoints it activates the custom dialer and disables security.
func ChooseDialOpts(endpoint string, opts ...grpc.DialOption) []grpc.DialOption {
	if strings.HasPrefix(endpoint, "unix://") {
		return append([]grpc.DialOption{
			grpc.WithDialer(GRPCDialer),
			grpc.WithInsecure(),
		},
			opts...)
	}
	return opts
}