/*
Copyright 2025 Vitor Bari.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validator

import (
	"context"
	"net"
	"time"
)

// TimeoutDialer provides a network dialer with configurable timeout
type TimeoutDialer struct {
	Timeout   time.Duration
	KeepAlive time.Duration
}

// DialContext connects to the address on the named network using the provided context
func (d *TimeoutDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	timeout := d.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second // default
	}

	keepAlive := d.KeepAlive
	if keepAlive == 0 {
		keepAlive = 30 * time.Second // default
	}

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: keepAlive,
	}

	return dialer.DialContext(ctx, network, address)
}
