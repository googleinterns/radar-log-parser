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
		"Android": []string{"Assert", "Error", "Warning", "Info", "Debug", "Verbose"}}
	log_levels_map = map[string]map[string]string{"Ios": map[string]string{"Critical": "C", "Error": "E", "Warning": "W", "Notice": "N", "Info": "I", "Debug": "D", "Trace": "T"},
		"Android": map[string]string{"Assert": "A", "Error": "E", "Warning": "W", "Info": "I", "Debug": "D", "Verbose": "V"}}
	log_levels_rgx = map[string]map[string]string{"Ios": map[string]string{"start": "", "end": ""},
		"Android": map[string]string{"start": "", "end": ""}}
)

func LogReport(w http.ResponseWriter, r *http.Request, fullLogDetails FullDetails) {
	switch file := r.URL.Path[len("/report/"):]; file {
	case FullLogDetails.Analysis_details.FileName:
		logContent := FullLogDetails.Analysis_details.RawLog
		err := r.ParseMultipartForm(10 << 20)
		if err == nil {
			level := r.FormValue("selectedLevel")
			if level != "All" {
				logContent = GetLogLeveldetails(FullLogDetails.Analysis_details.Platform, level, FullLogDetails.Analysis_details.RawLog)
			}
		}
		template.Must(template.New("details.html").Funcs(template.FuncMap{"detailType": func() string { return "Log" }, "countLine": func(content string) int { return len(strings.Split(content, "\n")) }}).ParseFiles("templates/details.html")).Execute(w, struct {
			Rawlog    string
			LogLevels []string
		}{
			logContent,
			Log_levels[FullLogDetails.Analysis_details.Platform],
		})
	case "events":
		GetImportantEvents(&CfgFile, FullLogDetails.Analysis_details.RawLog)
		ev_lines := make([]int, len(fullLogDetails.ImportantEvents), len(fullLogDetails.ImportantEvents))
		for line, _ := range fullLogDetails.ImportantEvents {
			ev_lines = append(ev_lines, line)
		}
		sort.Ints(ev_lines)
		template.Must(template.New("events.html").Funcs(template.FuncMap{}).ParseFiles("templates/events.html")).Execute(w, struct {
			MatchLines []int
			Events     map[int]string
		}{
			ev_lines,
			FullLogDetails.ImportantEvents,
		})

	default:
		if file[:7] == "Details" {
			issue_name := r.URL.Path[len("/report/Details/"):]
			details, ok := FullLogDetails.GroupedIssues[issue_name]
			if ok {
				template.Must(template.New("details.html").Funcs(template.FuncMap{"detailType": func() string { return "Group" }, "countLine": func(content string) int { return len(strings.Split(content, "\n")) }}).ParseFiles("templates/details.html")).Execute(w, details)
			} else {

				issue_num, _ := strconv.Atoi(FullLogDetails.Analysis_details.Issues[issue_name]["Number"])
				details, hightlight := nonGroupDetails(FullLogDetails.Analysis_details.RawLog, FullLogDetails.NonGroupedIssues[issue_name], issue_num)
				detail := struct {
					Highlight map[int]bool
					Details   []string
				}{
					hightlight, details,
				}
				template.Must(template.New("details.html").Funcs(template.FuncMap{"detailType": func() string { return "nonGroup" }, "countLine": func(content string) int { return len(strings.Split(content, "\n")) }}).ParseFiles("templates/details.html")).Execute(w, detail)

			}
		} else {
			template.Must(template.New("details.html").Funcs(template.FuncMap{"detailType": func() string { return "Log" }, "countLine": func(content string) int { return len(strings.Split(content, "\n")) }}).ParseFiles("templates/details.html")).Execute(w, FullLogDetails.Analysis_details.SpecificProcess[file])

		}
	}

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
	lev_rgx_comp, err := regexp.Compile(log_levels_rgx[platform]["start"] + log_levels_map[platform][level] + log_levels_rgx[platform]["end"])
	if err != nil {
		return ""
	}
	return strings.Join(lev_rgx_comp.FindAllString(fContent, -1), "\n")
}
