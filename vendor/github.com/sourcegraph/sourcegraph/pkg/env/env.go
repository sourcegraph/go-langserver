package env

import (
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	log15 "gopkg.in/inconshreveable/log15.v2"
)

var descriptions = make(map[string]string)
var locked = false
var env = expvar.NewMap("env")

var (
	// MyName represents the name of the current process.
	MyName, envVarName = findName()
	Version            = Get("VERSION", "dev", "the version of the packaged app, usually set by Dockerfile")
	LogLevel           = Get("SRC_LOG_LEVEL", "dbug", "upper log level to restrict log output to (dbug, info, warn, error, crit)")
	LogFormat          = Get("SRC_LOG_FORMAT", "logfmt", "log format (logfmt, condensed)")
	InsecureDev, _     = strconv.ParseBool(Get("INSECURE_DEV", "false", "Running in insecure dev (local laptop) mode"))
)

var (
	// DebugOut is os.Stderr if LogLevel includes dbug
	DebugOut io.Writer
	// InfoOut is os.Stderr if LogLevel includes info
	InfoOut io.Writer
	// WarnOut is os.Stderr if LogLevel includes warn
	WarnOut io.Writer
	// ErrorOut is os.Stderr if LogLevel includes error
	ErrorOut io.Writer
	// CritOut is os.Stderr if LogLevel includes crit
	CritOut io.Writer
)

// findName returns the name of the current process, that being the
// part of argv[0] after the last slash if any, and also the lowercase
// letters from that, suitable for use as a likely key for lookups
// in things like shell environment variables which can't contain
// hyphens.
func findName() (string, string) {
	// Environment variable names can't contain, for instance, hyphens.
	origName := filepath.Base(os.Args[0])
	name := strings.Replace(origName, "-", "_", -1)
	if name == "" {
		name = "unknown"
	}
	return origName, name
}

func init() {
	lvl, _ := log15.LvlFromString(LogLevel)
	lvlFilterStderr := func(maxLvl log15.Lvl) io.Writer {
		// Note that log15 values look like e.g. LvlCrit == 0, LvlDebug == 4
		if lvl > maxLvl {
			return ioutil.Discard
		}
		return os.Stderr
	}
	DebugOut = lvlFilterStderr(log15.LvlDebug)
	InfoOut = lvlFilterStderr(log15.LvlInfo)
	WarnOut = lvlFilterStderr(log15.LvlWarn)
	ErrorOut = lvlFilterStderr(log15.LvlError)
	CritOut = lvlFilterStderr(log15.LvlCrit)
}

// Get returns the value of the given environment variable. It also registers the description for
// PrintHelp. Calling Get with the same name twice causes a panic. Get should only be called on
// package initialization. Calls at a later point will cause a panic if Lock was called before.
//
// This should be used for only *internal* environment values. User-visible configuration should be
// added to the Config struct in the github.com/sourcegraph/sourcegraph/config package.
func Get(name string, defaultValue string, description string) string {
	if locked {
		panic("env.Get has to be called on package initialization")
	}

	if _, ok := descriptions[name]; ok {
		panic(fmt.Sprintf("%q already registered", name))
	}

	if defaultValue != "" {
		description = fmt.Sprintf("%s (default: %q)", description, defaultValue)
	}
	descriptions[name] = description

	// Allow per-process override. For instance, SRC_LOG_LEVEL_repo_updater would
	// apply to repo-updater, but not to anything else.
	perProg := name + "_" + envVarName
	value, ok := os.LookupEnv(perProg)
	if !ok {
		value, ok = os.LookupEnv(name)
		if !ok {
			value = defaultValue
		}
	}
	env.Set(name, jsonStringer(value))
	return value
}

type jsonStringer string

func (s jsonStringer) String() string {
	v, _ := json.Marshal(s)
	return string(v)
}

// Lock makes later calls to Get fail with a panic. Call this at the beginning of the main function.
func Lock() {
	locked = true
}

// PrintHelp prints a list of all registered environment variables and their descriptions.
func PrintHelp() {
	var names []string
	for name := range descriptions {
		names = append(names, name)
	}
	sort.Strings(names)

	log.Print("Environment variables:")
	for _, name := range names {
		log.Printf("  %-40s %s", name, descriptions[name])
	}
}

// HandleHelpFlag looks at the first CLI argument. If it is "help", "-h" or "--help", then it calls
// PrintHelp and exits.
func HandleHelpFlag() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "help", "-h", "--help":
			PrintHelp()
			os.Exit(0)
		}
	}
}
