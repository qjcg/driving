// Generate reports from RedHat Training survey files.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/gonum/stat"
	"github.com/hashicorp/logutils"
)

// NPS returns the NPS score given numbers of promoters (>= 9/10),
// passives (>= 7/10) & detractors (>= 0/10).
//
// The score can range from -100 (everybody is a detractor) to 100 (everybody
// is a promoter). An NPS that is positive (i.e., greater than zero) is felt to
// be good, and an NPS of +50 is excellent.
// Ref: https://en.wikipedia.org/wiki/Net_Promoter
func NPS(promoters, passives, detractors int) float64 {
	return float64(promoters-detractors) / float64(promoters+passives+detractors) * 100
}

// Report represents the final average scores and comments result of evaluating Surveys.
type Report struct {
	Responses int

	CurriculumAvg  float64
	InstructorAvg  float64
	EnvironmentAvg float64
	OverallAvg     float64
	NPS            float64

	CurriculumComments  map[string][]string
	InstructorComments  map[string][]string
	EnvironmentComments map[string][]string
	OverallComments     map[string][]string
}

// Survey represents a course survey response for an individual learner.
type Survey struct {
	Country    string
	Course     string
	CourseVer  string `json:"course_ver"`
	Email      string
	FoundVer   string `json:"found_ver"`
	Instructor string
	Language   string
	Modality   string
	Name       string
	Progress   string

	Q1508 string // Do you want to be contacted by Red Hat to discuss your training experience?

	// CURRICULUM (5 = strongly agree, 1 = strongly disagree, N/A)
	Q207 int    `json:",string"` // The student guide was accurate and had the right amount of detail
	Q208 int    `json:",string"` // The course had a logical structure and covered relevant subject matter
	Q209 int    `json:",string"` // The labs adequately reinforced the topics discussed in class
	Q210 int    `json:",string"` // The course allowed sufficient time to adequately cover the material
	Q508 string // Comments: Curriculum

	// INSTRUCTOR (5 = strongly agree, 1 = strongly disagree, N/A)
	Q306 int    `json:",string"` // The instructor demonstrated expertise in the topics taught
	Q307 int    `json:",string"` // The instructor showed evidence of strong preparation
	Q308 int    `json:",string"` // The instructor made concepts and tasks clear
	Q320 int    `json:",string"` // The instructor effectively managed classroom interaction and student participation
	Q310 int    `json:",string"` // The instructor provided accurate and helpful answers to questions
	Q318 string // Comments: Instructor

	// ONSITE ONLY?
	// CLASSROOM FACILITY (5 = strongly agree, 1 = strongly disagree, N/A)
	//Q611 int `json:",string"` // The computers and network were sufficient for the class
	//Q609 int `json:",string"` // The room and facility were comfortable
	//Q612 int `json:",string"` // The facility staff were hospitable
	//Q610 string //  Comments: Facility

	// VT ONLY?
	// LEARNING ENVIRONMENT (5 = strongly agree, 1 = strongly disagree, N/A)
	Q1901 string // I tested my connection and systems prior to the start of the course
	Q1002 int    `json:",string"` // Pre-class support was effective, responsive and accessible
	Q1003 int    `json:",string"` // The performance of the audio conferencing system was adequate
	Q1004 int    `json:",string"` // The performance of the web conferencing system was adequate
	Q1005 int    `json:",string"` // The performance of lab exercises was adequate
	Q1907 string // Comments: Learning Environment

	// OVERALL
	// Please tell us your overall rating of this training event
	//(5 = strongly positive, 1 = strongly negative, N/A)
	Q311 int `json:",string"`

	// How likely would you be to recommend Red Hat Training to a friend or
	// colleague in need of similar training?
	// (10 extremely likely, 1 extremely unlikely, N/A)
	Q410 int    `json:",string"`
	Q403 string // Comments: Overall

	// ADDITIONAL QUESTIONS (Yes / No)
	Q109 string // Did you meet the course prerequisites for this class?
	Q105 string // Did you complete Red Hats online skills assessment before enrolling in this class?
	Q111 string // Are you better prepared now than before class to maximize the value of your RedHat products?
	Q112 string // Are you more likely now than before class to explore the adoption of new RedHat technologies?
	Q113 string // Are your IT projects involving Red Hat technologies more likely to succeed after completing this training?
	Q101 string

	// YOU AND YOUR COMPANY
	Q1101 string // Which of the following best describes your job title? (select one)
	Q1201 string // Which of the following industries best classifies your company? (select one)
	Q1801 string // When was the last time you took Red Hat training from Red Hat (or from one of our authorized partners)?
	Q1401 string // Tell us about your company&#39;s or organization&#39;s current relationship to Red Hat (Select the item that most closely matches)
	Q1701 string // What is the primary reason you are taking this Red Hat training?

	StartDate  string `json:"start_date"`
	Subscript  string
	SurveyDate string
	SurveyVer  string `json:"survey_ver"`
}

func main() {
	debug := flag.Bool("d", false, "print debugging output")
	flag.Parse()

	// Set up levelled logging.
	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO"},
		MinLevel: logutils.LogLevel("INFO"),
		Writer:   os.Stdout,
	}
	if *debug {
		filter.MinLevel = logutils.LogLevel("DEBUG")
	}
	log.SetOutput(filter)

	// An io.Reader that we will read our survey data from (os.Stdin by
	// default).
	var r io.Reader
	r = os.Stdin

	// If file arguments are passed on the command line, use these files as
	// source of survey data.
	if flag.NArg() > 0 {
		var files []io.Reader
		for _, filename := range flag.Args() {
			f, err := os.Open(filename)
			if err != nil {
				log.Fatalf("[INFO] Error opening file: %s\n", err)
			}
			defer f.Close()

			files = append(files, f)
		}

		r = io.MultiReader(files...)
	}

	surveyBytes, err := TxtToJSON(r)
	if err != nil {
		log.Fatalf("[INFO] Error converting txt to JSON: %s\n", err)
	}
	log.Printf("[DEBUG] surveyBytes:\n%s\n", surveyBytes)

	var surveys []*Survey
	dec := json.NewDecoder(bytes.NewReader(surveyBytes))
	for dec.More() {
		var s Survey
		err := dec.Decode(&s)
		if err != nil {
			log.Printf("[DEBUG] Decode error: %s\n", err)
		}
		surveys = append(surveys, &s)
	}

	fmt.Printf(NewReport(surveys).String())
}

// NewReport returns a new Report from a slice of Surveys.
func NewReport(surveys []*Survey) Report {
	var curriculumAvgsSum,
		instructorAvgsSum,
		environmentAvgsSum,
		overallAvgsSum float64
	var promoters, passives, detractors int

	for _, s := range surveys {
		curriculumAvgsSum += float64(s.Q207+s.Q208+s.Q209+s.Q210) / 4.0
		instructorAvgsSum += float64(s.Q306+s.Q307+s.Q308+s.Q320) / 4.0
		environmentAvgsSum += float64(s.Q1002+s.Q1003+s.Q1004+s.Q1005) / 4.0
		overallAvgsSum += float64(s.Q311)

		// Tally NPS variables.
		switch {
		case s.Q410 >= 9:
			promoters++
		case s.Q410 >= 7:
			passives++
		case s.Q410 >= 0:
			detractors++
		}
	}

	//mean, std := stat.MeanStdDev([]float64{}, nil)
	stat.MeanStdDev([]float64{}, nil)

	n := float64(len(surveys))
	return Report{
		Responses:      int(n),
		NPS:            NPS(promoters, passives, detractors),
		CurriculumAvg:  curriculumAvgsSum / n,
		InstructorAvg:  instructorAvgsSum / n,
		EnvironmentAvg: environmentAvgsSum / n,
		OverallAvg:     overallAvgsSum / n,
	}
}

// String returns a string containing the Report's data.
func (r Report) String() string {
	return fmt.Sprintf("%-11s %3d\n%-11s %6.2f\n%-11s %6.2f\n%-11s %6.2f\n%-11s %6.2f\n%-11s %6.2f\n",
		"Responses", r.Responses,
		"Curriculum", r.CurriculumAvg,
		"Instructor", r.InstructorAvg,
		"Environment", r.EnvironmentAvg,
		"Overall", r.OverallAvg,
		"NPS", r.NPS,
	)
}

// TxtToJSON converts the native .txt survey format to JSON, for use as a
// convenient intermediate representation before ultimate unmarshalling to a
// Survey struct.
func TxtToJSON(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer

	// Begin a new JSON object.
	buf.Write([]byte("{\n"))

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		// Validate input. Every .txt survey line should have an "=".
		if !strings.Contains(line, "=") {
			return buf.Bytes(), errors.New("invalid input")
		}

		// Survey delimiter. End last JSON object and begin a new one.
		if line == "=" {
			buf.Write([]byte("}\n{\n"))
			continue
		}

		// Data lines.
		// Remove dash from question name (Q12-15 → Q1215) for
		// unmarshalling, since idiomatic Go variable names don't use
		// dashes.
		question := strings.Split(line, "=")
		question[0] = strings.Replace(question[0], "-", "", -1)
		line = fmt.Sprintf("  \"%s\": \"%s\",\n", question[0], question[1])

		// survey_ver is (always?) the last field for each survey
		// record. Do not print a trailing comma.
		if strings.Contains(line, "survey_ver") {
			line = strings.Replace(line, ",\n", "\n", 1)
		}
		buf.Write([]byte(line))
	}

	// Remove extraneous trailing brace.
	jsonBytes := bytes.TrimRight(buf.Bytes(), "{\n")

	return jsonBytes, nil
}
