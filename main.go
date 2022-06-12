package main

import (
	"os"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
)

// EDIT THIS FUNCTION!
// This is the main logic. rl is the input `ResourceList` which has the `FunctionConfig` and `Items` fields.
// You can modify the `Items` and add result information to `rl.Result`.
func setImage(rl *fn.ResourceList) (bool, error) {
	si := SetImage{}

	err, ok := si.Config(rl)
	if !ok {
		return false, err
	}

	if err != nil {
		rl.Results = append(rl.Results, fn.ErrorConfigObjectResult(err, rl.FunctionConfig))
		return true, nil
	}
	err = si.Transform(rl)
	if err != nil {
		return false, err
	}
	rl.Results = append(rl.Results, si.SdkResults()...)
	return true, nil
}

func main() {
	// CUSTOMIZE IF NEEDED
	// `AsMain` accepts a `ResourceListProcessor` interface.
	// You can explore other `ResourceListProcessor` structs in the SDK or define your own.
	if err := fn.AsMain(fn.ResourceListProcessorFunc(setImage)); err != nil {
		os.Exit(1)
	}
}
