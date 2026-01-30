// Package main implements a CLI tool for monitoring Virginia Tech course sections
// and notifying users when seats become available.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/resend/resend-go/v2"
)

// timetableUrl is the Virginia Tech timetable endpoint for course searches
const timetableUrl = "https://selfservice.banner.vt.edu/ssb/HZSKVTSC.P_ProcRequest"

// ==================================
// Configuration
// ==================================

// Config holds the runtime configuration for the course monitor
type Config struct {
	CRNs          []string `json:"crns"`          // Course Reference Number(s) to monitor
	Email         string   `json:"email"`         // Email address for notifications (optional)
	CheckInterval int      `json:"checkInterval"` // Time between availability checks
	Term          string   `json:"term"`          // Term code (e.g., 202601 = Spring 2026)
	Campus        string   `json:"campus"`        // Campus code (0 = Blacksburg)
}

type CourseStatus struct {
	CRN   string
	Name  string
	Found bool
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse config file: %w", err)
	}

	// set defaults
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 30
	}
	if cfg.Campus == "" {
		cfg.Campus = "0"
	}
	if cfg.Term == "" {
		cfg.Term = "202601"
	}

	if len(cfg.CRNs) == 0 {
		return Config{}, fmt.Errorf("no CRNs specified in config")
	}

	return cfg, nil
}

// parseFlags parses command-line flags and returns a Config.
// It exits with an error if required flags are missing.
// func parseFlags() Config {
// 	// flag command-line args return pointers
// 	crnPtr := flag.String("crn", "", "The CRN of the course section to monitor (required)")
// 	waitPtr := flag.Int("wait", 30, "Seconds to wait between checks")
// 	emailPtr := flag.String("email", "", "Email for notification")
// 	flag.Parse()
//
// 	if *crnPtr == "" {
// 		log.Fatal("Error: -crn flag is required")
// 	}
//
// 	return Config{
// 		CRN:           *crnPtr,
// 		CheckInterval: time.Duration(*waitPtr) * time.Second,
// 		Campus:        "0",
// 		Term:          "202601",
// 		Email:         *emailPtr,
// 	}
// }

// buildPayload constructs the form data for a timetable search request.
// If openOnly is true, results are filtered to sections with available seats.
func (c Config) buildPayload(crn string, openOnly bool) url.Values {
	// Initialize as a standard Go map
	rawMap := map[string][]string{
		"CAMPUS":           {c.Campus},
		"TERMYEAR":         {c.Term},
		"CORE_CODE":        {"AR%"},
		"subj_code":        {"%"},
		"SCHDTYPE":         {"%"},
		"CRSE_NUMBER":      {""},
		"crn":              {crn},
		"sess_code":        {"%"},
		"BTN_PRESSED":      {"FIND class sections"},
		"inst_name":        {""},
		"disp_comments_in": {""},
	}
	if openOnly {
		rawMap["open_only"] = []string{"on"}
	}
	// Convert the map to the url.Values type so it can be passed into http methods
	payload := url.Values(rawMap)

	return payload
}

// ====================================
// HTTP / Scraping
// ====================================

// fetchDocument sends a POST request to the given URL and parses the response as HTML.
// Returns the parsed document or an error if the request fails or returns non-200 status.
func fetchDocument(targetUrl string, payload url.Values) (*goquery.Document, error) {
	resp, err := http.PostForm(targetUrl, payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d %s", resp.StatusCode, resp.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	return doc, err
}

// checkSectionOpen checks if the configured course section has available seats.
// Returns true if the section appears in open-only search results.
func checkSectionOpen(cfg Config, crn string) (bool, error) {
	payload := cfg.buildPayload(crn, true)
	doc, err := fetchDocument(timetableUrl, payload)
	if err != nil {
		return false, err
	}

	table := doc.Find(".dataentrytable").Text()
	return strings.Contains(table, crn), nil
}

// getCourseName retrieves the course title for the configured CRN.
// Returns an error if the CRN is not found in the timetable.
func getCourseName(cfg Config, crn string) (string, error) {
	payload := cfg.buildPayload(crn, false)
	doc, err := fetchDocument(timetableUrl, payload)
	if err != nil {
		return "", err
	}

	var courseName string
	doc.Find(".dataentrytable tr").Each(func(i int, row *goquery.Selection) {
		// check if the row contains the target crn
		if strings.Contains(row.Find("td:nth-child(1)").Text(), crn) {
			// the course title is in the 3rd td cell
			courseName = strings.TrimSpace(row.Find("td:nth-child(3)").Text())
		}
	})

	if courseName == "" {
		return "", fmt.Errorf("course not found for CRN: %s", crn)
	}

	return courseName, nil
}

// =================================
// Notifications
// =================================

// sendEmail sends a notification email using the Resend API.
// Requires RESEND_API_KEY environment varialbe to be set.
func sendEmail(to, subject, body string) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("RESEND_API_KEY not set")
	}

	client := resend.NewClient(apiKey)

	params := &resend.SendEmailRequest{
		From:    "onboarding@resend.dev",
		To:      []string{to},
		Subject: subject,
		Text:    body,
		// Html: "<p>Hello, World!</p>",
	}

	_, err := client.Emails.Send(params)
	return err
}

// ===================================
// Main
// ===================================

func main() {
	configPathPtr := flag.String("config", "config.json", "Path to config file")
	flag.Parse()

	cfg, err := loadConfig(*configPathPtr)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// initialize course statuses - filter out invalid CRNs
	var courses []CourseStatus
	for _, crn := range cfg.CRNs {
		name, err := getCourseName(cfg, crn)
		if err != nil {
			log.Printf("Warning: couldn't get name for CRN %s: %v. Removing from monitor list.", crn, err)
			continue
		}
		courses = append(courses, CourseStatus{CRN: crn, Name: name, Found: false})
		fmt.Printf("Monitoring: %s (CRN: %s)\n", name, crn)
	}

	if len(courses) == 0 {
		log.Fatalf("No valid CRNs to monitor")
	}

	fmt.Println()

	remaining := len(courses)
	interval := time.Duration(cfg.CheckInterval) * time.Second
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	for attempt := 1; remaining > 0; attempt++ {
		checkTime := time.Now().Format("15:04:05")

		for i := range courses {
			if courses[i].Found {
				continue
			}

			fmt.Printf("\r%s [Attempt %d] Checking %s...                              ",
				spinner[attempt%len(spinner)], attempt, courses[i].CRN)

			open, err := checkSectionOpen(cfg, courses[i].CRN)
			if err != nil {
				fmt.Printf("\r✗ [%s] Error checking %s: %v\n", checkTime, courses[i].CRN, err)
				continue
			}

			if open {
				courses[i].Found = true
				remaining--

				msg := fmt.Sprintf("OPEN SEAT: %s (CRN: %s)", courses[i].Name, courses[i].CRN)
				fmt.Printf("\r\n🎉 %s\n", msg)
				sendEmail(cfg.Email, "VT Course Section Open!", msg)
			}

			time.Sleep(500 * time.Millisecond) // Small delay between requests
		}

		if remaining == 0 {
			fmt.Println("\nAll courses found!")
			return
		}

		// Animate spinner while waiting
		waitUntil := time.Now().Add(interval)
		i := 0
		for time.Now().Before(waitUntil) {
			timeLeft := time.Until(waitUntil).Round(time.Second)
			found := len(courses) - remaining
			fmt.Printf("\r%s [%s] %d/%d found. Next check in %v...          ",
				spinner[i%len(spinner)], checkTime, found, len(courses), timeLeft)
			time.Sleep(100 * time.Millisecond)
			i++
		}
	}
}
