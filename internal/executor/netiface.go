package executor

import (
	"context"
	"fmt"
	"net"
)

// runNetIface implements `kopsc net iface`: show network interfaces and IPs.
func runNetIface(ctx context.Context, req BuiltinRequest) error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("获取网卡列表失败: %w", err)
	}

	for _, iface := range ifaces {
		// Skip loopback if not requested
		if iface.Flags&net.FlagLoopback != 0 {
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

		// IP addresses
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			fmt.Fprintf(req.Stdout, "    IP:   %s\n", addr.String())
		}

		// Flags
		fmt.Fprintf(req.Stdout, "    标志: %s\n", iface.Flags.String())
		fmt.Fprintln(req.Stdout)
	}

	return nil
}
