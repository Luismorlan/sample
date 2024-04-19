package utils

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rnr-capital/newsfeed-backend/utils/dotenv"
	"gonum.org/v1/gonum/stat/distuv"
)

// ContainsString returns true iff the provided string slice hay contains string
// needle.
func ContainsString(hay []string, needle string) bool {
	for _, str := range hay {
		if str == needle {
			return true
		}
	}
	return false
}

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// This function will return random string of target length consisting
// alphabetic characters (lowercase) and number.
func RandomAlphabetString(length int) string {
	rand.Seed(time.Now().UnixNano())
	chars := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

// for testing
func parseGQLTimeString(str string) (time.Time, error) {
	return time.Parse(time.RFC3339, str)
}

func serializeGQLTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

func AreJSONsEqual(s1, s2 string) (bool, error) {

	if len(s1) == 0 && len(s2) == 0 {
		// both invalid json, return true
		return true, nil
	} else if len(s1) == 0 || len(s2) == 0 {
		return false, nil
	}

	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal([]byte(s1), &o1)
	if err != nil {
		return false, errors.Wrap(err, "Error mashalling string s1 in AreJSONsEqual()")
	}
	err = json.Unmarshal([]byte(s2), &o2)
	if err != nil {
		return false, errors.Wrap(err, "Error mashalling string s2 in AreJSONsEqual()")
	}

	return reflect.DeepEqual(o1, o2), nil
}

func StringSlicesContainSameElements(s1, s2 []string) bool {
	sort.Strings(s1)
	sort.Strings(s2)
	return reflect.DeepEqual(s1, s2)
}

func StringifyBoolean(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func GetRandomDataCollectorFunctionName() string {
	return "data_collector_" + RandomAlphabetString(8)
}

func GetRandomNumberInRangeStandardDeviation(mean float64, radius float64) float64 {
	// Use 3 standard diviation, which has the 99.7% probability to succeed.
	deviation := float64(3)
	for {
		dist := distuv.UnitNormal
		num := dist.Rand()
		if num <= deviation && num >= -deviation {
			return num*radius/deviation + mean
		}
	}
}

func IsProdEnv() bool {
	return os.Getenv("NEWSMUX_ENV") == dotenv.ProdEnv
}

func TextToMd5Hash(input string) (string, error) {
	hasher := md5.New()
	_, err := hasher.Write([]byte(input))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// ImmediatePrintError logs the given error along with the file path and line number where the error occurred.
// It uses the runtime.Caller function to retrieve the file path and line number of the caller.
// If the error is not nil, it prints the error message along with the file path and line number.
// The function then returns the original error.
func ImmediatePrintError(err error) error {
	if err != nil {
		// notice that we're using 1, so it will actually log the where
		// the error happened, 0 = this function, we don't want that.
		_, fn, line, _ := runtime.Caller(1)
		fmt.Printf("\n[%s:%d] %v\n", fn, line, err)
	}
	return err
}

func GetUrlExtNameWithDot(url string) string {
	return filepath.Ext(url)
}

func FallbackString(primary, fallback string) string {
	if primary == "" {
		return fallback
	}
	return primary
}

func ParseDate(date string) (time.Time, error) {
	return time.Parse("2006-01-02", date)
}
