/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2017-2023 WireGuard LLC. All Rights Reserved.
 */

package conn

import (
	"bytes"
	"testing"

	"golang.org/x/net/ipv6"
)

// TestSplitCoalescedMessagesKeepsControl is a regression test for the S-UI
// Next patch in splitCoalescedMessages. With UDP GRO the batch is read into the
// last slots and split towards the front; every split datagram must keep the
// original control messages (IP_PKTINFO / IPV6_PKTINFO), otherwise sticky
// sockets lose the local destination address and replies leave from the wrong
// source address.
func TestSplitCoalescedMessagesKeepsControl(t *testing.T) {
	const gsoSize = 4
	control := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

	msgs := make([]ipv6.Message, 3)
	for i := range msgs {
		msgs[i].Buffers = [][]byte{make([]byte, 64)}
		msgs[i].OOB = make([]byte, 0, 64)
	}
	// The coalesced read lands in the last slot: two full segments and a short tail.
	src := &msgs[2]
	src.N = copy(src.Buffers[0], []byte("aaaabbbbcc"))
	src.OOB = append(src.OOB[:0], control...)
	src.NN = len(control)

	n, err := splitCoalescedMessages(msgs, 2, func([]byte) (int, error) { return gsoSize, nil })
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("split into %d messages, want 3", n)
	}
	want := []string{"aaaa", "bbbb", "cc"}
	for i := 0; i < n; i++ {
		if got := string(msgs[i].Buffers[0][:msgs[i].N]); got != want[i] {
			t.Errorf("message %d payload = %q, want %q", i, got, want[i])
		}
		if got := msgs[i].OOB[:msgs[i].NN]; !bytes.Equal(got, control) {
			t.Errorf("message %d control = %v, want %v", i, got, control)
		}
	}
}
