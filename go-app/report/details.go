package report

import (
	"html/template"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var (
	Log_levels = map[string][]string{"Ios": []string{"Critical", "Error", "Warning", "Notice", "Info", "Debug", "Trace"},
		"my-android-bucket": []string{"Assert", "Error", "Warning", "Info", "Debug", "Verbose"}}
	log_levels_map = map[string]map[string]string{"Ios": map[string]string{"Critical": "C", "Error": "E", "Warning": "W", "Notice": "N", "Info": "I", "Debug": "D", "Trace": "T"},
		"my-android-bucket": map[string]string{"Assert": "A", "Error": "E", "Warning": "W", "Info": "I", "Debug": "D", "Verbose": "V"}}
	log_levels_rgx = map[string]map[string]string{"Ios": map[string]string{"start": "", "end": ""},
		"my-android-bucket": map[string]string{"start": "(?m)^(?:0[1-9]|1[0-2])-(?:0[1-9]|(?:1|2)[0-9]|3(?:0|1))\\s(?:(?:(?:0|1)[0-9])|(?:2[0-3])):[0-5][0-9]:[0-5][0-9]\\.\\d{3}(?:\\s)*\\d{4,5}(?:\\s)*\\d{4,5}\\s", "end": "\\s.*"}}
)

func LogReport(w http.ResponseWriter, r *http.Request, fullLogDetails *FullDetails, cfgFile *Config) {
	file := r.URL.Path[len("/report/"):]
	switch file {
	case fullLogDetails.Analysis_details.FileName:
		loadRawLog(w, r, fullLogDetails)
	case "events":
		loadEvents(w, r, fullLogDetails, cfgFile)
	default:
		if file[:7] == "Details" {
			issue_name := r.URL.Path[len("/report/Details/"):]
			_, ok := fullLogDetails.GroupedIssues[issue_name]
			if ok {
				loadGroupDetails(w, issue_name, fullLogDetails)
			} else {
				loadNonGroupDetails(w, issue_name, fullLogDetails)
			}
		} else {
			loadSpecificLogs(w, file, fullLogDetails)
		}
	}

}
func loadSpecificLogs(w http.ResponseWriter, file string, fullLogDetails *FullDetails) {
	FuncMap := template.FuncMap{
		"detailType": func() string { return "SpecificLog" },
		"countLine":  CountLine,
	}
	detail_template, err := template.New("details.html").Funcs(FuncMap).ParseFiles("templates/details.html")
	template := template.Must(detail_template, err)
	template.Execute(w, fullLogDetails.Analysis_details.SpecificProcess[file])
}
func loadGroupDetails(w http.ResponseWriter, issue_name string, fullLogDetails *FullDetails) {
	FuncMap := template.FuncMap{
		"detailType": func() string { return "Group" },
		"countLine":  CountLine,
	}
	detail_template, err := template.New("details.html").Funcs(FuncMap).ParseFiles("templates/details.html")
	template := template.Must(detail_template, err)
	template.Execute(w, fullLogDetails.GroupedIssues[issue_name])
}
func loadNonGroupDetails(w http.ResponseWriter, issue_name string, fullLogDetails *FullDetails) {
	issue_num, _ := strconv.Atoi(fullLogDetails.Analysis_details.Issues[issue_name]["Number"])
	hightlight := make(map[int]bool) //index=> true = must be highlight
	details := nonGroupDetails(fullLogDetails.Analysis_details.RawLog, fullLogDetails.NonGroupedIssues[issue_name], issue_num, hightlight)
	FuncMap := template.FuncMap{
		"detailType": func() string { return "nonGroup" },
		"countLine":  CountLine,
	}
	detail_template, err := template.New("details.html").Funcs(FuncMap).ParseFiles("templates/details.html")
	template := template.Must(detail_template, err)
	template.Execute(w, struct {
		Highlight map[int]bool
		Details   []string
	}{
		hightlight, details,
	})
}
func loadEvents(w http.ResponseWriter, r *http.Request, fullLogDetails *FullDetails, cfgFile *Config) {
	fullLogDetails.ImportantEvents = make(map[int]string)
	logs_size := getImportantEvents(cfgFile, fullLogDetails.Analysis_details.RawLog, fullLogDetails.ImportantEvents)
	ev_lines := make([]int, 0, len(fullLogDetails.ImportantEvents))
	for line, _ := range fullLogDetails.ImportantEvents {
		ev_lines = append(ev_lines, line)
	}
	sort.Ints(ev_lines)
	event_logs := make([]string, 0, len(fullLogDetails.ImportantEvents))
	contentSlice := strings.Split(fullLogDetails.Analysis_details.RawLog, "\n")
	for _, line := range ev_lines {
		event_logs = append(event_logs, contentSlice[line])
	}
	event_template, err := template.New("events.html").Funcs(template.FuncMap{"add": func(x, y int) int {
		return x + y
	}, "substract": func(x, y int) int {
		return x - y
	}}).ParseFiles("templates/events.html")
	template := template.Must(event_template, err)
	template.Execute(w, struct {
		MatchLines []int
		Events     map[int]string
		LogSize    int
		EventLogs  []string
	}{
		ev_lines,
		fullLogDetails.ImportantEvents,
		logs_size,
		event_logs,
	})
}
func loadRawLog(w http.ResponseWriter, r *http.Request, fullLogDetails *FullDetails) {
	FuncMap := template.FuncMap{
		"detailType": func() string { return "RawLog" },
		"countLine":  CountLine,
	}
	detail_template, err := template.New("details.html").Funcs(FuncMap).ParseFiles("templates/details.html")
	template := template.Must(detail_template, err)
	template.Execute(w, struct {
		Rawlog    string
		LogLevels []string
	}{
		fullLogDetails.Analysis_details.RawLog,
		Log_levels[fullLogDetails.Analysis_details.Platform],
	})
}
func CountLine(content string) int {
	return len(strings.Split(content, "\n"))
}
func nonGroupDetails(fContent string, nonGroupedIssues map[string]bool, num_issue int, hightlight map[int]bool) []string {
	details := make([]string, 0, num_issue*2+1)
	last_found_index := 0
	contentLines := strings.Split(fContent, "\n")
	found_issue := 0
	for index, line := range contentLines {
		if nonGroupedIssues[line] {
			if last_found_index != index {
				details = append(details, strings.Join(contentLines[last_found_index:index], "\n"))
				hightlight[len(details)-1] = false
			}
			details = append(details, line)
			last_found_index = index + 1
			hightlight[len(details)-1] = true
			found_issue += 1

			if found_issue == num_issue {
				details = append(details, strings.Join(contentLines[last_found_index:], "\n"))
				hightlight[len(details)-1] = false
				break
			}
		}

	}
	return details
}
func getImportantEvents(cfgFile *Config, fContent string, importantEvents map[int]string) int {
	if len(cfgFile.ImportantEvents) < 1 {
		return 0
	}
	contentMap := make(map[string]int)
	contentSlice := strings.Split(fContent, "\n")
	for index, line := range contentSlice {
		contentMap[line] = index
	}
	var waitGroup sync.WaitGroup
	var mutex sync.Mutex
	waitGroup.Add(len(cfgFile.ImportantEvents))
	for ev, ev_rgx := range cfgFile.ImportantEvents {
		go func(ev string, ev_rgx string) {
			ev_rgx_comp, err := regexp.Compile(ev_rgx)
			if err != nil {
				waitGroup.Done()
				return
			}
			ev_content := ev_rgx_comp.FindAllString(fContent, -1)
			if len(ev_content) > 0 {
				mutex.Lock()
				for _, match_ev := range ev_content {
					importantEvents[contentMap[match_ev]] = ev
				}
				mutex.Unlock()
			}
			waitGroup.Done()
		}(ev, ev_rgx)
	}
	waitGroup.Wait()
	return len(contentSlice)
}
func GetLogLeveldetails(platform string, level string, fContent string) string {
	level_rgx := log_levels_rgx[platform]["start"] + log_levels_map[platform][level] + log_levels_rgx[platform]["end"]
	lev_rgx_comp, err := regexp.Compile(level_rgx)
	if err != nil {
		return ""
	}
	return strings.Join(lev_rgx_comp.FindAllString(fContent, -1), "\n")
}
