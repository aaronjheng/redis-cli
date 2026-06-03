package redis

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gomodule/redigo/redis"
)

type Printer struct {
	Raw bool
}

func (p *Printer) Print(result any) {
	p.PrintIndenting(result, "", false)
}

func (p *Printer) PrintIndenting(result any, prefix string, forceRaw bool) {
	switch val := result.(type) {
	case []any:
		if p.isRawMode(forceRaw) {
			p.printListRaw(val, forceRaw)
		} else {
			p.printListFormatted(val, prefix, forceRaw)
		}
	default:
		fmt.Fprintf(os.Stdout, "%s\n", p.toValueString(result, forceRaw))
	}
}

func (p *Printer) printListRaw(items []any, forceRaw bool) {
	for _, item := range items {
		switch subItem := item.(type) {
		case []any:
			p.PrintIndenting(subItem, "", forceRaw)
		default:
			fmt.Fprintf(os.Stdout, "%s\n", p.toValueString(subItem, forceRaw))
		}
	}
}

func (p *Printer) printListFormatted(items []any, prefix string, forceRaw bool) {
	spacer := strings.Repeat(" ", len(prefix))

	for idx, item := range items {
		switch subItem := item.(type) {
		case []any:
			newprefix := prefix + " " + strconv.FormatInt(int64(idx+1), 10) + ")"
			p.PrintIndenting(subItem, newprefix, forceRaw)
		default:
			if idx == 0 {
				fmt.Fprintf(os.Stdout, "%s %d) %s\n", prefix, idx+1, p.toValueString(item, forceRaw))
			} else {
				fmt.Fprintf(os.Stdout, "%s %d) %s\n", spacer, idx+1, p.toValueString(item, forceRaw))
			}
		}
	}
}

func (p *Printer) isRawMode(forceRaw bool) bool {
	return p.Raw || forceRaw
}

func (p *Printer) toValueString(value any, forceRaw bool) string {
	switch val := value.(type) {
	case redis.Error:
		if p.isRawMode(forceRaw) {
			return val.Error()
		}

		return "(error) " + val.Error()
	case int64:
		if p.isRawMode(forceRaw) {
			return strconv.FormatInt(val, 10)
		}

		return "(integer) " + strconv.FormatInt(val, 10)
	case string:
		return val
	case []byte:
		if p.isRawMode(forceRaw) {
			return string(val)
		}

		return "\"" + string(val) + "\""
	case nil:
		return "nil"
	}

	return ""
}
