# Patched copy of github.com/sagernet/wireguard-go

This directory is an unmodified copy of
`github.com/sagernet/wireguard-go@v0.0.2-beta.1.0.20260224074747-506b7631853c`
(the version required by sing-box v1.13.14), taken from proxy.golang.org, plus
the patch below. It is wired in through the `replace` directive in the
repository's `go.mod`.

## conn: keep control messages when splitting GRO-coalesced datagrams

With UDP GRO enabled (the default on Linux kernels that support it),
`StdNetBind.receiveIP` reads into the last slots of the message batch and
`splitCoalescedMessages` copies each datagram towards the front. The copy only
carried `Buffers`, `N` and `Addr`, not `OOB`/`NN`, so `getSrcFromControl` saw no
`IP_PKTINFO`/`IPV6_PKTINFO` and every endpoint lost its sticky source address.

Replies then left from the kernel's preferred source address instead of the
address the peer sent to. On a host with more than one address this breaks the
WireGuard handshake, for example:

- a LAN client reaching the router's public IPv6 address (hairpin): the reply
  uses the LAN address and the client drops it;
- IPv6 privacy (temporary) addresses rotating: the reply uses the newest
  temporary address while the client still talks to the previous one.

Upstream `golang.zx2c4.com/wireguard` has the same code. The patch copies the
control messages into each split datagram (`conn/bind_std.go`) and adds
`conn/bind_std_sticky_test.go` as a regression test.

Drop this directory and the `replace` directive once sing-box depends on a
wireguard-go release that contains an equivalent fix.
