package utils

// Disable profiler because we don't use it.

// import (
// 	. "github.com/rnr-capital/newsfeed-backend/utils/flag"
// 	Logger "github.com/rnr-capital/newsfeed-backend/utils/log"
// 	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
// )

// func init() {
// 	// Datadog profiler

// 	env := "development"
// 	if IsProdEnv() {
// 		env = "production"
// 	}

// 	if err := profiler.Start(
// 		profiler.WithService(ServiceName),
// 		profiler.WithEnv(env),
// 		profiler.WithProfileTypes(
// 			profiler.CPUProfile,
// 			profiler.HeapProfile,
// 			// The profiles below are disabled by
// 			// default to keep overhead low, but
// 			// can be enabled as needed.
// 			// profiler.BlockProfile,
// 			// profiler.MutexProfile,
// 			// profiler.GoroutineProfile,
// 		),
// 	); err != nil {
// 		Logger.Log.Fatal(err)
// 	}
// }

// // Stop profiler, OK to be closed multiple times
// func CloseProfiler() {
// 	// Datadog profiler
// 	profiler.Stop()
// }
