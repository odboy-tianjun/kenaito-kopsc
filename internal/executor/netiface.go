package executor

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// runNetIface implements `kopsc net iface`: show network interfaces with IPv4 addresses.
func runNetIface(ctx context.Context, req BuiltinRequest) error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("获取网卡列表失败: %w", err)
	}

	for _, iface := range ifaces {
		// Skip loopback
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// Get IP addresses
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		// Filter IPv4 addresses only
		var ipv4Addrs []string
		for _, addr := range addrs {
			addrStr := addr.String()
			// IPv4 addresses contain '.' but not ':'
			if strings.Contains(addrStr, ".") && !strings.Contains(addrStr, ":") {
				ipv4Addrs = append(ipv4Addrs, addrStr)
			}
		}

		// Skip if no IPv4 address
		if len(ipv4Addrs) == 0 {
			continue
		}

		// Status
		status := "DOWN"
		if iface.Flags&net.FlagUp != 0 {
			status = "UP"
		}

		fmt.Fprintf(req.Stdout, "%s: <%s> mtu %d\n", iface.Name, status, iface.MTU)

		// MAC address
		if len(iface.HardwareAddr) > 0 {
			fmt.Fprintf(req.Stdout, "    MAC:  %s\n", iface.HardwareAddr.String())
		}

		// IPv4 addresses only
		for _, addr := range ipv4Addrs {
			fmt.Fprintf(req.Stdout, "    IPv4: %s\n", addr)
		}

		fmt.Fprintln(req.Stdout)
	}

	return nil
}
