package cli

import (
	"flag"
	"fmt"
	"strings"
)

func parseFlagSet(fs *flag.FlagSet, args []string) error {
	reordered, err := reorderFlagArgs(fs, args)
	if err != nil {
		return err
	}
	return fs.Parse(reordered)
}

func reorderFlagArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	flagArgs := []string{}
	posArgs := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			posArgs = append(posArgs, args[i:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			if strings.Contains(arg, "=") {
				flagArgs = append(flagArgs, arg)
				continue
			}
			name := strings.TrimLeft(arg, "-")
			if name == "" {
				posArgs = append(posArgs, arg)
				continue
			}
			flagValue := fs.Lookup(name)
			if flagValue == nil {
				flagArgs = append(flagArgs, arg)
				continue
			}
			if isBoolFlag(flagValue) {
				flagArgs = append(flagArgs, arg)
				continue
			}
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag needs an argument: %s", arg)
			}
			flagArgs = append(flagArgs, arg, args[i+1])
			i++
			continue
		}
		posArgs = append(posArgs, arg)
	}
	return append(flagArgs, posArgs...), nil
}

func isBoolFlag(flagValue *flag.Flag) bool {
	boolFlag, ok := flagValue.Value.(interface{ IsBoolFlag() bool })
	return ok && boolFlag.IsBoolFlag()
}
