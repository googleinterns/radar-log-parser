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

func LogReport(w http.ResponseWriter, r *http.Request, fullLogDetails FullDetails) {
	file := r.URL.Path[len("/report/"):]
	switch file {
	case FullLogDetails.Analysis_details.FileName:
		loadRawLog(w, r, fullLogDetails)
	case "events":
		loadEvents(w, r, fullLogDetails)
	default:
		if file[:7] == "Details" {
			issue_name := r.URL.Path[len("/report/Details/"):]
			_, ok := FullLogDetails.GroupedIssues[issue_name]
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
func loadSpecificLogs(w http.ResponseWriter, file string, fullLogDetails FullDetails) {
	FuncMap := template.FuncMap{
		"detailType": func() string { return "SpecificLog" },
		"countLine":  CountLine,
	}
	detail_template, err := template.New("details.html").Funcs(FuncMap).ParseFiles("templates/details.html")
	template := template.Must(detail_template, err)
	template.Execute(w, FullLogDetails.Analysis_details.SpecificProcess[file])
}
func loadGroupDetails(w http.ResponseWriter, issue_name string, fullLogDetails FullDetails) {
	FuncMap := template.FuncMap{
		"detailType": func() string { return "Group" },
		"countLine":  CountLine,
	}
	detail_template, err := template.New("details.html").Funcs(FuncMap).ParseFiles("templates/details.html")
	template := template.Must(detail_template, err)
	template.Execute(w, FullLogDetails.GroupedIssues[issue_name])
}
func loadNonGroupDetails(w http.ResponseWriter, issue_name string, fullLogDetails FullDetails) {
	issue_num, _ := strconv.Atoi(FullLogDetails.Analysis_details.Issues[issue_name]["Number"])
	details, hightlight := nonGroupDetails(FullLogDetails.Analysis_details.RawLog, FullLogDetails.NonGroupedIssues[issue_name], issue_num)
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
func loadEvents(w http.ResponseWriter, r *http.Request, fullLogDetails FullDetails) {
	GetImportantEvents(&CfgFile, FullLogDetails.Analysis_details.RawLog)
	ev_lines := make([]int, len(fullLogDetails.ImportantEvents), len(fullLogDetails.ImportantEvents))
	for line, _ := range fullLogDetails.ImportantEvents {
		ev_lines = append(ev_lines, line)
	}
	sort.Ints(ev_lines)
	event_template, err := template.New("events.html").Funcs(template.FuncMap{}).ParseFiles("templates/events.html")
	template := template.Must(event_template, err)
	template.Execute(w, struct {
		MatchLines []int
		Events     map[int]string
	}{
		ev_lines,
		FullLogDetails.ImportantEvents,
	})
}
func loadRawLog(w http.ResponseWriter, r *http.Request, fullLogDetails FullDetails) {
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
		FullLogDetails.Analysis_details.RawLog,
		Log_levels[FullLogDetails.Analysis_details.Platform],
	})
}
func CountLine(content string) int {
	return len(strings.Split(content, "\n"))
}
func nonGroupDetails(fContent string, nonGroupedIssues map[string]bool, num_issue int) ([]string, map[int]bool) {
	details := make([]string, 0, num_issue*2+1)
	hightlight := map[int]bool{ //index=> true = must be highlight
	}
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
	return details, hightlight
}
func GetImportantEvents(cfgFile *Config, fContent string) {
	var importantEvents = make(map[int]string)
	if len(cfgFile.ImportantEvents) < 1 {
		return
	}
	contentMap := make(map[string]int)
	contentSlice := strings.Split(fContent, "\n")
	for index, line := range contentSlice {
		contentMap[line] = index
	}
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(cfgFile.ImportantEvents))
	for ev, ev_rgx := range cfgFile.ImportantEvents {
		go func(ev string, ev_rgx string) {
			ev_rgx_comp, err := regexp.Compile(ev_rgx)
			if err != nil {
				waitGroup.Done()
				return
			}
			ev_content := ev_rgx_comp.FindString(fContent)
			if ev_content != "" {
				importantEvents[contentMap[ev_content]] = ev
			}
			waitGroup.Done()
		}(ev, ev_rgx)
	}
	waitGroup.Wait()
	FullLogDetails.ImportantEvents = importantEvents
	return
}
func GetLogLeveldetails(platform string, level string, fContent string) string {
	level_rgx := log_levels_rgx[platform]["start"] + log_levels_map[platform][level] + log_levels_rgx[platform]["end"]
	lev_rgx_comp, err := regexp.Compile(level_rgx)
	if err != nil {
		return ""
	}
	return strings.Join(lev_rgx_comp.FindAllString(fContent, -1), "\n")
}
