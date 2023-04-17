package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/apstndb/spannerplanviz/queryplan"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalln(err)
	}
}

const jsonSnippetLen = 140

func run(ctx context.Context) error {
	flag.Parse()

	b, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	stats, _, err := queryplan.ExtractQueryPlan(b)
	if err != nil {
		var collapsedStr string
		if len(b) > jsonSnippetLen {
			collapsedStr = "(collapsed)"
		}
		return fmt.Errorf("invalid input at protoyaml.Unmarshal:\nerror: %w\ninput: %.*s%s", err, jsonSnippetLen, strings.TrimSpace(string(b)), collapsedStr)
	}

	qp := queryplan.New(stats.GetQueryPlan().GetPlanNodes())

	for _, row := range qp.PlanNodes() {
		var msgs []string
		switch {
		case row.GetDisplayName() == "Filter":
			msgs = append(msgs, "Expensive operator Filter can't utilize index: Can't you use Filter Scan with Seek Condition?")
		case strings.Contains(row.GetDisplayName(), "Minor Sort"):
			msgs = append(msgs, "Expensive operator Minor Sort is cheaper than Sort but it may be not optimal: Can't you use the same order with the index?")
		case strings.Contains(row.GetDisplayName(), "Sort"):
			msgs = append(msgs, "Expensive operator Sort: Can't you use the same order with the index?")
		}
		for _, childLink := range row.GetChildLinks() {
			var msg string
			switch {
			case childLink.GetType() == "Residual Condition":
				msg = "Expensive Residual Condition: Try to translate it to Scan Condition"
			}
			if msg != "" {
				msgs = append(msgs, fmt.Sprintf("%v: %v", childLink.GetType(), msg))
			}
		}
		for k, v := range row.GetMetadata().AsMap() {
			var msg string
			switch {
			case k == "Full scan" && v == "true":
				msg = "Expensive execution full scan: Do you really want full scan?"
			case k == "iterator_type" && v == "Hash":
				msg = fmt.Sprintf("Expensive execution Hash %s: Can't you modify to use Stream %s?", row.GetDisplayName(), row.GetDisplayName())
			case k == "join_type" && v == "Hash":
				msg = fmt.Sprintf("Expensive execution Hash %s: Can't you modify to use Cross Apply or Merge Join?", row.GetDisplayName())
			}
			if msg != "" {
				msgs = append(msgs, fmt.Sprintf("%v=%v: %v", k, v, msg))
			}
		}
		if len(msgs) > 0 {
			fmt.Printf("%v: %v\n", row.GetIndex(), queryplan.NodeTitle(row))
			for _, msg := range msgs {
				fmt.Printf("    %v\n", msg)
			}
		}
	}
	return nil
}
