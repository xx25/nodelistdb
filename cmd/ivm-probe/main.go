// Command ivm-probe exercises the VModem/IVM protocol tester against one or
// more hosts and prints the detected protocol variant, conformance and any
// identified software. It talks to the network directly and needs no database,
// so it is the quickest way to validate the tester against live nodes.
//
// By default it places a real outgoing Virtual Modem Protocol call against
// silent (i.e. genuine VMODEM) endpoints, which rings the remote sysop's mailer
// and reads back their identity over EMSI. That only works when this host is
// reachable inbound, because the answering node dials the data channel back to
// us; use -vmp=false to classify without calling.
//
// Usage:
//
//	ivm-probe [flags] <host> <port> [expectedAddress]
//	ivm-probe [flags]            # probe a small built-in set of known IVM nodes
package main

import (
	"context"
	"flag"
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
	var (
		vmp         = flag.Bool("vmp", true, "place a real VMP call against silent endpoints")
		listenHost  = flag.String("listen-host", "", "bind address for the VMP data channel")
		prefPort    = flag.Int("preferred-port", 14592, "data-channel port to try first (VMODEM's own first choice)")
		portMin     = flag.Int("port-min", 0, "low bound of the fallback port range (0 = ephemeral)")
		portMax     = flag.Int("port-max", 0, "high bound of the VMP data-channel port range")
		ringTimeout = flag.Duration("ring-timeout", 45*time.Second, "how long to let the remote ring its mailer")
		timeout     = flag.Duration("timeout", 15*time.Second, "per-connection timeout")
		ourAddress  = flag.String("our-address", "2:5001/5001", "FTN address to present in the EMSI handshake")
		debug       = flag.Bool("debug", os.Getenv("IVM_DEBUG") != "", "verbose protocol logging")
	)
	flag.Parse()

	var targets []target
	switch args := flag.Args(); {
	case len(args) >= 2:
		port, err := strconv.Atoi(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid port %q: %v\n", args[1], err)
			os.Exit(2)
		}
		addr := ""
		if len(args) >= 3 {
			addr = args[2]
		}
		targets = []target{{args[0], port, addr}}
	case len(args) == 0:
		targets = []target{
			{"f360.n221.z2.fidonet.fi", 3141, "2:221/360"}, // expect: vmp
			{"fido.bajer.cz", 3141, "2:423/81"},            // expect: vmp
			{"bbs.roonsbbs.hu", 3141, "2:371/52"},          // expect: vmp
			{"scbbs.nsupdate.info", 60177, "2:201/137"},    // expect: emsi-raw
			{"tfb-bbs.org", 3141, "3:54/0"},                // expect: emsi-telnet
			{"185.22.236.179", 2030, "2:420/0"},            // expect: emsi-telnet (FrontDoor)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: ivm-probe [flags] <host> <port> [expectedAddress]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	tester := protocols.NewVModemTesterWithInfo(*timeout, *ourAddress, "NodelistDB Probe", "Tester", "Testland")
	tester.SetDebug(*debug)
	if *vmp {
		tester.EnableVMPCalls(*listenHost, *prefPort, *portMin, *portMax, *ringTimeout)
	}

	fmt.Printf("%-26s %-7s %-13s %-11s %-8s %s\n", "HOST", "PORT", "VARIANT", "CONFORMANT", "ms", "SOFTWARE / DETAIL")
	fmt.Println("--------------------------------------------------------------------------------------------------------")
	for _, t := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), *ringTimeout+*timeout*3)
		r, _ := tester.Test(ctx, t.host, t.port, t.addr).(*protocols.VModemTestResult)
		cancel()
		if r == nil {
			fmt.Printf("%-26s %-7d (no result)\n", t.host, t.port)
			continue
		}
		info := r.Software
		if r.Detail != "" {
			info = fmt.Sprintf("%s | %s", info, r.Detail)
		}
		if r.Error != "" {
			info = r.Error
		}
		fmt.Printf("%-26s %-7d %-13s %-11v %-8d %s\n", t.host, t.port, r.Variant, r.Conformant, r.ResponseMs, info)
		if r.SystemName != "" {
			fmt.Printf("%-26s %-7s system:    %s\n", "", "", r.SystemName)
		}
		if r.Sysop != "" {
			fmt.Printf("%-26s %-7s sysop:     %s\n", "", "", r.Sysop)
		}
		if r.Location != "" {
			fmt.Printf("%-26s %-7s location:  %s\n", "", "", r.Location)
		}
		if r.CallOutcome != "" {
			fmt.Printf("%-26s %-7s vmp call:  %s\n", "", "", r.CallOutcome)
		}
		if len(r.Addresses) > 0 {
			fmt.Printf("%-26s %-7s addresses: %v (expected present: %v)\n", "", "", r.Addresses, r.AddressValid)
		}
	}
}
