package test

import (
	"fmt"
	"github.com/wmbest2/android/apk"
	"regexp"
	"strconv"
	"strings"
	"time"
    "github.com/wmbest2/rats_server/rats"
)

const (
	NUM_TESTS = iota
	STREAM
	ID
	TEST
	CLASS
	CURRENT
	STACK
	CODE
)

type instToken struct {
	Timestamp time.Time
	Type      int
	Value     []byte
}

func tokenForLine(line [][]byte) *instToken {
	token := &instToken{Timestamp: time.Now()}
	if string(line[1]) == "CODE" {
		token.Type = CODE
		token.Value = line[2]
	} else {
		switch string(line[3]) {
		case "numTests":
			token.Type = NUM_TESTS
		case "stream":
			token.Type = STREAM
		case "stack":
			token.Type = STACK
		case "id":
			token.Type = ID
		case "test":
			token.Type = TEST
		case "current":
			token.Type = CURRENT
		case "class":
			token.Type = CLASS
		}

		token.Value = line[4]
	}
	return token
}

func processLastToken(test *TestCase, token *instToken) bool {
	if token == nil {
		return false
	}
	switch token.Type {
	case TEST:
		test.Name = strings.TrimSpace(string(token.Value))
	case CLASS:
		test.Classname = strings.TrimSpace(string(token.Value))
	case STACK:
		test.Stack = string(token.Value) + "\n"
	default:
		return false
	}
	return true
}

func parseInstrumentation(suite *TestSuite, in chan interface{}, out chan *TestSuite) {
	instrumentCheck := regexp.MustCompile("INSTRUMENTATION_STATUS(?:(?:_(CODE): (.*))|(?:: ([^=\n]*)=(.*)))")
	var currentTest *TestCase
	var lastToken *instToken
	var startTime, endTime time.Time
	var v interface{}

	ok := true

	for {
		if !ok {
			break
		}

		switch v, ok = <-in; v.(type) {
		case []byte:
			if instrumentCheck.Match(v.([]byte)) {

				if currentTest == nil {
					currentTest = &TestCase{}
					suite.Tests++
				}

				vals := instrumentCheck.FindSubmatch(v.([]byte))
				lastToken = tokenForLine(vals)

				if suite.Tests == 0 && lastToken.Type == NUM_TESTS {
					suite.Tests, _ = strconv.Atoi(string(lastToken.Value))
				}

				processLastToken(currentTest, lastToken)
				if lastToken.Type == CODE && string(lastToken.Value) == "1" {
					startTime = lastToken.Timestamp
				} else if lastToken.Type == CODE {
					endTime = lastToken.Timestamp
					switch string(lastToken.Value) {
					case "-2":
						currentTest.Failure = &currentTest.Stack
						suite.Failures++
					case "-1":
						currentTest.Error = &currentTest.Stack
						suite.Errors++
					}

					currentTest.Time = endTime.Sub(startTime).Seconds()
					suite.Time += currentTest.Time
					suite.TestCases = append(suite.TestCases, currentTest)
					currentTest = nil
					lastToken = nil
				}
			} else {
				if lastToken != nil && lastToken.Type == STACK {
					currentTest.Stack += string(v.([]byte))
				}
			}
		case error:
			fmt.Println(v.(error))
		}
	}
	out <- suite
}

func RunTests(manifest *apk.Manifest) *TestSuites {
	out := make(chan *TestSuite)
    suites := &TestSuites{Success: true}

    devices := <-rats.GetDevices()
	for _, d := range devices {
		go RunTest(d, manifest, out)
	}

	for _ = range devices {
		suite := <-out
		suites.TestSuites = append(suites.TestSuites, suite)
		suites.Time += suite.Time
        suites.Success = suites.Success && suite.Failures == 0 && suite.Errors == 0
	}

	return suites
}

func LogTestSuite(device *rats.Device, manifest *apk.Manifest, out chan *TestSuite) {
	testRunner := fmt.Sprintf("%s/%s", manifest.Package, manifest.Instrument.Name)
	in := device.Exec("shell", "am", "instrument", "-r", "-e", "log", "true","-w", testRunner)
    suite := TestSuite{Device: device, Hostname: device.Serial, Name: device.String()}
	parseInstrumentation(&suite, in, out)
}

func RunTest(device *rats.Device, manifest *apk.Manifest, out chan *TestSuite) {
	testRunner := fmt.Sprintf("%s/%s", manifest.Package, manifest.Instrument.Name)
	in := device.Exec("shell", "am", "instrument", "-r", "-w", testRunner)
	suite := TestSuite{Device: device, Hostname: device.Serial, Name: device.String()}
	parseInstrumentation(&suite, in, out)
}
