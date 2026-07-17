// Command ivm-probe exercises the VModem/IVM protocol tester against one or
// more hosts and prints the detected protocol variant, conformance and any
// identified software. It talks to the network directly and needs no database,
// so it is the quickest way to validate the tester against live nodes.
//
// Usage:
//
//	ivm-probe <host> <port> [expectedAddress]
//	ivm-probe            # probe a small built-in set of known IVM nodes
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/nodelistdb/internal/testing/protocols"
)

type target struct {
	host string
	port int
	addr string
}

func main() {
	debug := os.Getenv("IVM_DEBUG") != ""
	var targets []target

	if len(os.Args) >= 3 {
		port, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid port %q: %v\n", os.Args[2], err)
			os.Exit(2)
		}
		addr := ""
		if len(os.Args) >= 4 {
			addr = os.Args[3]
		}
		targets = []target{{os.Args[1], port, addr}}
	} else if len(os.Args) == 1 {
		targets = []target{
			{"fido.bajer.cz", 3141, "2:423/81"},         // expect: vmp
			{"bbs.roonsbbs.hu", 3141, "2:371/52"},       // expect: vmp
			{"scbbs.nsupdate.info", 60177, "2:201/137"}, // expect: emsi-raw
			{"tfb-bbs.org", 3141, "3:54/0"},             // expect: emsi-telnet
			{"185.22.236.179", 2030, "2:420/0"},         // expect: emsi-telnet (FrontDoor)
		}
	} else {
		fmt.Fprintln(os.Stderr, "usage: ivm-probe <host> <port> [expectedAddress]")
		os.Exit(2)
	}

	tester := protocols.NewVModemTesterWithInfo(15*time.Second, "2:5001/5001", "NodelistDB Probe", "Tester", "Testland")
	tester.SetDebug(debug)

	fmt.Printf("%-24s %-7s %-13s %-10s %-9s %s\n", "HOST", "PORT", "VARIANT", "CONFORMANT", "ms", "SOFTWARE / DETAIL")
	fmt.Println("--------------------------------------------------------------------------------------------------------")
	for _, t := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		r, _ := tester.Test(ctx, t.host, t.port, t.addr).(*protocols.VModemTestResult)
		cancel()
		if r == nil {
			fmt.Printf("%-24s %-7d (no result)\n", t.host, t.port)
			continue
		}
		info := r.Software
		if r.SystemName != "" {
			info = fmt.Sprintf("%s | %s", info, r.SystemName)
		}
		if r.Detail != "" {
			info = fmt.Sprintf("%s | %s", info, r.Detail)
		}
		if r.Error != "" {
			info = r.Error
		}
		fmt.Printf("%-24s %-7d %-13s %-10v %-9d %s\n", t.host, t.port, r.Variant, r.Conformant, r.ResponseMs, info)
		if len(r.Addresses) > 0 {
			fmt.Printf("%-24s %-7s addresses: %v\n", "", "", r.Addresses)
		}
	}
}
