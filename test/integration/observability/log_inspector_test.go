package observability

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

const contextLines = 5 //number of lines to show before and after the suspicious line

var allowlistedMessages = []string{ //list of messages that are allowed to be in the logs
	"AllocationSetRange has empty AssetSet in accumulation",
}

func TestOpenCostLogs(t *testing.T) { //use kubernetes to fetch all logs from containers in the opencost pods inside the opencost namespace
	cmd := exec.Command(
		"kubectl", "logs", //get containers logs
		"-n", "opencost", //look inside the kubernetes namespace for opencost
		"-l", "app.kubernetes.io/instance=opencost", //look for the opencost pods with this label
		"--all-containers", //If the pod has multiple containers, get logs from all of them
		"--tail=-1",        //show all log lines
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("kubectl cannot reach OpenCost logs: %v", err) //if there is an error, skip the test
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n") // line is only 1 and blank then it is nil
	if len(lines) == 1 && lines[0] == "" {
		lines = nil //if the line is only 1 and blank then it is nil
	}
	t.Logf("fetched %d log lines from OpenCost", len(lines)) //log the number of lines fetched

	var badLines []string //collect all the suspicious lines
	for _, line := range lines {
		if isSuspicious(line) { //if the line is suspicious
			if isAllowlisted(line) { //if the line is allowlisted then skip it
				continue
			}
			badLines = append(badLines, line)
		}
	}
	t.Logf("found %d suspicious lines", len(badLines))

	if len(badLines) > 0 {
		for i, line := range lines {
			if !isSuspicious(line) || isAllowlisted(line) {
				continue
			}
			t.Errorf("unexpected log issue at line %d:\n%s", i+1, formatContext(lines, i))
		}
		t.Fatalf("found %d suspicious log lines", len(badLines))
	}
}

func stripAnsi(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`) //used to remove ANSI color codes
	return re.ReplaceAllString(s, "")          //return the string with the ANSI color codes removed
}

func isSuspicious(line string) bool {
	clean := stripAnsi(line)

	if strings.Contains(clean, "panic:") { //if the line contains "panic:" then it is Panic
		return true
	}
	if strings.Contains(clean, "goroutine ") { //if the line contains "goroutine " then it is a Stack Trace
		return true
	}
	if strings.Contains(clean, " ERR ") { //if the line contains " ERR " then it is an Error log
		return true
	}
	return false
}

func isAllowlisted(line string) bool {
	clean := stripAnsi(line)
	for _, pattern := range allowlistedMessages {
		if strings.Contains(clean, pattern) { //if the line contains the allowlisted message then it is allowlisted
			return true
		}
	}
	return false
}

func formatContext(lines []string, index int) string { //takes in a list of log lines and the index of suspicous line in that list
	start := index - contextLines //decide how many lines to show before the suspicious line
	if start < 0 {                //if the start is less than 0 then set it to 0
		start = 0
	}
	end := index + contextLines //decide how many lines to show after the suspicious line
	if end >= len(lines) {      //if the end is greater than the length of the lines then set it to the length of the lines - 1
		end = len(lines) - 1
	}

	var b strings.Builder //creates a string builder
	for i := start; i <= end; i++ {
		prefix := "  "
		if i == index {
			prefix = ">>" //if the line is the suspicious line then add ">>" to the prefix
		}
		b.WriteString(prefix)              //add the prefix to the string builder
		b.WriteString(stripAnsi(lines[i])) //add the line to the string builder
		b.WriteString("\n")                //add a new line to the string builder
	}
	return b.String() //return the string builder as a string
}
