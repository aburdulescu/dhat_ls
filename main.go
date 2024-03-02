package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"os"
	"runtime/debug"
	"strings"
	"text/tabwriter"
)

const usage = `Usage: dhat_ls [FLAGS] DHAT_FILE

Generate a report with all allocations recorded in the given DHAT output file.

By default, the generated report will be written to STDOUT as regular text.
Use -html to generate a HTML report.

Specific allocations can be ignored by using a ignore file.
A ignore file contains keywords(e.g. my_function) which will be searched in the
frame stack of all allocations.
If the frame stack of an allocation contains one of the keywords, that allocation
will not be added to the generated report.

The ignore file must contain a list of keywords separated by newline('\n').
Whitespaces(' ' and '\t') are trimmed from the start and end of the lines.
Empty lines and comment lines(which start with '#') are ignored.

FLAGS:
`

func main() {
	if err := mainErr(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func mainErr(args []string) error {
	fset := flag.NewFlagSet("root", flag.ContinueOnError)

	fset.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		fset.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
	}

	ignoreFile := fset.String("i", "", "`File` with keywords to ignored, one per line")
	outputHtml := fset.Bool("html", false, "Generate HTML output")
	printVersion := fset.Bool("version", false, "Print version")

	if err := fset.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *printVersion {
		version()
		return nil
	}

	if fset.NArg() != 1 {
		fset.Usage()
		return fmt.Errorf("need DHAT file")
	}

	ignoreList, err := parseIgnoreFile(*ignoreFile)
	if err != nil {
		return err
	}

	report, err := parseReport(fset.Arg(0))
	if err != nil {
		return err
	}

	if *outputHtml {
		fmt.Print(htmlHeader)
	}

	if *outputHtml {
		fmt.Printf("<br><pre>\n")
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, " \t \n")
	fmt.Fprintf(w, "Command:\t%s\n", report.Cmd)
	fmt.Fprintf(w, "PID:\t%d\n", report.PID)
	w.Flush()

	if *outputHtml {
		fmt.Printf("</pre><br><hr><br>\n")
	}

	allocCount := 1

	for i, pp := range report.ProgramPoints {
		if shouldIgnore(*report, i, ignoreList) {
			continue
		}

		if *outputHtml {
			fmt.Printf("<details><summary>Allocation #%d</summary><br><pre>\n", allocCount)
		} else {
			fmt.Printf("\n==== Allocation #%d ====\n", allocCount)
		}

		fmt.Printf(
			"Total: %d bytes in %d blocks, avg size %d bytes\n",
			pp.TotalBytes, pp.TotalBlocks, pp.TotalBytes/pp.TotalBlocks,
		)
		fmt.Printf("\n")

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)

		fmt.Fprintf(w, "#\tFrame\n")
		fmt.Fprintf(w, "-\t-----\n")

		allocCount++

		for j := len(pp.Frames) - 1; j >= 0; j-- {
			frame := report.GetFrame(pp.Frames[j])
			fmt.Fprintf(w, "%d\t", len(pp.Frames)-j)
			if *outputHtml {
				fmt.Fprintf(w, "%s\n", html.EscapeString(frame))
			} else {
				fmt.Fprintf(w, "%s\n", frame)
			}
		}

		w.Flush()

		if *outputHtml {
			fmt.Println("</pre></details><br>")
		}
	}

	if *outputHtml {
		fmt.Print(`
</body>
</html>
`)
	}

	return nil
}

func version() {
	bi, _ := debug.ReadBuildInfo()
	g := func(k string) string {
		for _, v := range bi.Settings {
			if v.Key == k {
				return v.Value
			}
		}
		return ""
	}
	fmt.Println("go     ", bi.GoVersion)
	fmt.Println("main   ", bi.Main.Version)
	if v := g("vcs.revision"); v != "" {
		fmt.Println("commit ", g("vcs.revision"))
	}
	if v := g("vcs.time"); v != "" {
		fmt.Println("time   ", g("vcs.time"))
	}
	if v := g("vcs.modified"); v != "" {
		fmt.Println("dirty  ", g("vcs.modified"))
	}
}

const htmlHeader = `
<!DOCTYPE html>

<html lang="en">

<head>

<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">

<style>
body {
font-size: 15px;
}
pre {
overflow-x: auto;
}

details > summary {
  padding: 2px 6px;
  width: 15em;
  background-color: #ddd;
  border: none;
  box-shadow: 3px 3px 4px black;
  cursor: pointer;
}

pre {
  border-radius: 0 0 10px 10px;
  background-color: #ddd;
  padding: 2px 6px;
  margin: 0;
  box-shadow: 3px 3px 4px black;
}

details[open] > summary {
  background-color: #ccf;
}

button {
  background-color: #ddd;
  font-size: 15px;
  width: 10%;
}

</style>

<title>DHAT allocations report</title>

</head>

<body>

<button id="btn-openall">Open All</button>
<button id="btn-closeall">Close All</button>
<hr>

<script>

document.getElementById("btn-openall").addEventListener("click", function(event) {
  const elements = document.querySelectorAll('details');
  elements.forEach(element => {
    element.open = true;
  });
});

document.getElementById("btn-closeall").addEventListener("click", function(event) {
  const elements = document.querySelectorAll('details');
  elements.forEach(element => {
    element.open = false;
  });
});

</script>

`

func parseReport(file string) (*Report, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var report Report
	if err := json.NewDecoder(f).Decode(&report); err != nil {
		return nil, err
	}
	return &report, nil
}

func parseIgnoreFile(file string) ([]string, error) {
	if file == "" {
		return nil, nil
	}

	ignoreList := make([]string, 0, 32)

	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(string(content), "\n") {
		line := strings.Trim(line, " \t")
		if line == "" {
			continue
		}
		if line[0] == '#' {
			continue
		}
		ignoreList = append(ignoreList, line)
	}

	return ignoreList, nil

}

func shouldIgnore(r Report, frame int, ignoreList []string) bool {
	for _, s := range ignoreList {
		if r.ProgramPointHasFrame(frame, s) {
			return true
		}
	}
	return false
}

type Report struct {
	// Version number of the format. Incremented on each
	// backwards-incompatible change. A mandatory integer.
	Version int `json:"dhatFileVersion"`

	// The invocation mode. A mandatory, free-form string.
	InvocationMode string `json:"mode"`

	// The verb used before above stack frames, i.e. "<verb> at {". A
	// mandatory string.
	StackFrameVerb string `json:"verb"`

	// Are block lifetimes recorded? Affects whether some other fields are
	// present. A mandatory boolean.
	BlockLifetimesRecorded bool `json:"bklt"`

	// Are block accesses recorded? Affects whether some other fields are
	// present. A mandatory boolean.
	BlockAccessesRecorded bool `json:"bkacc"`

	// Byte/bytes/blocks-position units. Optional strings. "byte", "bytes",
	// and "blocks" are the values used if these fields are omitted.
	ByteUnit   string `json:"bu,omitempty"`
	BytesUnit  string `json:"bsu,omitempty"`
	BlocksUnit string `json:"bksu,omitempty"`

	// Time units (individual and 1,000,000x). Mandatory strings.
	TimeUnit    string `json:"tu,omitempty"`
	MilTimeUnit string `json:"mtu,omitempty"`

	// The "short-lived" time threshold, measures in "tu"s.
	// - bklt=true: a mandatory integer.
	// - bklt=false: omitted.
	ShortLivedTimeThreshold int `json:"tuth"`

	// The executed command. A mandatory string.
	Cmd string `json:"cmd"`

	// The process ID. A mandatory integer.
	PID int `json:"pid"`

	// The time at the end of execution (t-end). A mandatory integer.
	TimeAtEnd int `json:"te"`

	// The time of the global max (t-gmax).
	// - bklt=true: a mandatory integer.
	// - bklt=false: omitted.
	TimeAtGlobalMax int `json:"tg"`

	// The program points. A mandatory array.
	ProgramPoints []ProgramPoint `json:"pps"`

	// Frame table. A mandatory array of strings.
	FramesTable []string `json:"ftbl"`
}

func (r Report) ProgramPointHasFrame(i int, s string) bool {
	for _, frame := range r.ProgramPoints[i].Frames {
		sym := strings.Split(r.FramesTable[frame], ": ")[1]
		if strings.Contains(sym, s) {
			return true
		}
	}
	return false
}

func (r Report) GetFrame(i int) string {
	return strings.Split(r.FramesTable[i], ": ")[1]
}

type ProgramPoint struct {
	// Total bytes and blocks. Mandatory integers.
	TotalBytes  int `json:"tb"`
	TotalBlocks int `json:"tbk"`

	// Total lifetimes of all blocks allocated at this PP.
	// - bklt=true: a mandatory integer.
	// - bklt=false: omitted.
	TotalLifetimesOfBlocks int `json:"tl"`

	// The maximum bytes and blocks for this PP.
	// - bklt=true: mandatory integers.
	// - bklt=false: omitted.
	MaxBytes  int `json:"mb"`
	MaxBlocks int `json:"mbk"`

	// The bytes and blocks at t-gmax for this PP.
	// - bklt=true: mandatory integers.
	// - bklt=false: omitted.
	BytesAtTgmax  int `json:"gb"`
	BlocksAtTgmax int `json:"gbk"`

	// The bytes and blocks at t-end for this PP.
	// - bklt=true: mandatory integers.
	// - bklt=false: omitted.
	BytesAtTend  int `json:"eb"`
	BlocksAtTend int `json:"ebk"`

	// The reads and writes of blocks for this PP.
	// - bkacc=true: mandatory integers.
	// - bkacc=false: omitted.
	ReadsOfBlocks  int `json:"rb"`
	WritesOfBlocks int `json:"wb"`

	// The exact accesses of blocks for this PP. Only used when all
	// allocations are the same size and sufficiently small. A negative
	// element indicates run-length encoding of the following integer.
	// E.g. `-3, 4` means "three 4s in a row".
	// - bkacc=true: an optional array of integers.
	// - bkacc=false: omitted.
	BlockAccesses []int `json:"acc"`

	// Frames. Each element is an index into the "ftbl" array below.
	// - All modes: A mandatory array of integers.
	Frames []int `json:"fs"`
}
