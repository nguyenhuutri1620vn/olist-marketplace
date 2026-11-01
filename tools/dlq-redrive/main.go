// Command dlq-redrive inspects and redrives dead-letter topics (AD-7):
//
//	dlq-redrive -topic order.events   copy order.events.dlq back to order.events
//	dlq-redrive -check                exit 1 if any *.dlq topic is not empty
//
// It reads the brokers from KAFKA_BROKERS. Redriving is safe because every
// consumer is idempotent.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/bootstrap"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/inbox"
)

// timeout bounds one run.
const timeout = 2 * time.Minute

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout))
}

// run returns the exit code: 0 done, 1 non-empty DLQ or failure, 2 usage.
func run(ctx context.Context, args []string, out io.Writer) int {
	fs := flag.NewFlagSet("dlq-redrive", flag.ContinueOnError)
	fs.SetOutput(out)
	topic := fs.String("topic", "", "redrive <topic>.dlq back to <topic>")
	check := fs.Bool("check", false, "exit 1 if any *.dlq topic holds records not yet redriven")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if (*topic == "") == !*check {
		fmt.Fprintln(out, "usage: dlq-redrive -topic <topic> | -check")
		return 2
	}
	if strings.HasSuffix(*topic, inbox.DLQSuffix) {
		fmt.Fprintf(out, "-topic is the original topic, not %s\n", *topic)
		return 2
	}
	brokers, err := bootstrap.KafkaBrokers()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if *check {
		nonEmpty, err := inbox.NonEmptyDLQs(ctx, brokers)
		if err != nil {
			fmt.Fprintln(out, "check DLQs:", err)
			return 1
		}
		if len(nonEmpty) == 0 {
			fmt.Fprintln(out, "every DLQ is empty")
			return 0
		}
		names := make([]string, 0, len(nonEmpty))
		for name := range nonEmpty {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			fmt.Fprintf(out, "%s: %d\n", name, nonEmpty[name])
		}
		fmt.Fprintln(out, "DLQs are not empty: inspect them, then make dlq-redrive TOPIC=<topic>")
		return 1
	}

	n, err := inbox.Redrive(ctx, brokers, *topic)
	if err != nil {
		fmt.Fprintf(out, "redrive %s: %v (redrove %d)\n", *topic, err, n)
		return 1
	}
	fmt.Fprintf(out, "redrove %d records from %s%s to %s\n", n, *topic, inbox.DLQSuffix, *topic)
	return 0
}
