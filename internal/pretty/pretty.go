// Package pretty implements pretty-print facilities.
package pretty

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tdp"
	"github.com/gotd/td/tg"
)

type Invoker struct {
	Next tg.Invoker
}

func formatObject(input interface{}) string {
	o, ok := input.(tdp.Object)
	if !ok {
		// Handle tg.*Box values.
		rv := reflect.Indirect(reflect.ValueOf(input))
		for i := 0; i < rv.NumField(); i++ {
			if v, ok := rv.Field(i).Interface().(tdp.Object); ok {
				return formatObject(v)
			}
		}

		return fmt.Sprintf("%T (not object)", input)
	}
	return tdp.Format(o)
}

func (i Invoker) InvokeRaw(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	fmt.Println("→", formatObject(input))
	start := time.Now()
	if err := i.Next.InvokeRaw(ctx, input, output); err != nil {
		fmt.Println("←", err)
		return err
	}

	fmt.Printf("← (%s) %s\n", time.Since(start).Round(time.Millisecond), formatObject(output))

	return nil
}
