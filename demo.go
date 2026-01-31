package main

import (
	"time"
)

// RunDemo runs a scripted demo for recording GIFs/videos
func RunDemo() {
	// Demo courses
	courses := []CourseStatus{
		{CRN: "13466", Name: "Data Structures and Algorithms", Found: false},
		{CRN: "13472", Name: "Computer Systems", Found: false},
	}
	demoEmail := "student@vt.edu"

	// Display banner and config
	PrintBanner()
	PrintConfigBox(len(courses), demoEmail, 30, "202601")

	// Simulate fetching courses
	PrintFetchingHeader()
	time.Sleep(500 * time.Millisecond)
	for _, course := range courses {
		PrintCourseFound(course.CRN, course.Name)
		time.Sleep(400 * time.Millisecond)
	}

	PrintDivider()

	// Monitoring loop simulation
	remaining := len(courses)

	for attempt := 1; remaining > 0; attempt++ {
		checkTime := time.Now().Format("15:04:05")

		// Check each course
		for i := range courses {
			if courses[i].Found {
				continue
			}

			// Show checking status with spinner animation
			for spin := 0; spin < 15; spin++ {
				PrintCheckingStatus(spin, attempt, courses[i].CRN)
				time.Sleep(100 * time.Millisecond)
			}

			// Simulate finding a seat on specific attempts
			foundSeat := false
			if courses[i].CRN == "13466" && attempt >= 2 {
				foundSeat = true
			} else if courses[i].CRN == "13472" && attempt >= 3 {
				foundSeat = true
			}

			if foundSeat {
				courses[i].Found = true
				remaining--

				PrintSeatAvailable(courses[i].Name, courses[i].CRN)
				time.Sleep(300 * time.Millisecond)
				PrintEmailSent(demoEmail)
				time.Sleep(500 * time.Millisecond)
			}
		}

		if remaining == 0 {
			break
		}

		// Animate waiting spinner
		waitDuration := 3 * time.Second
		waitUntil := time.Now().Add(waitDuration)
		spin := 0
		for time.Now().Before(waitUntil) {
			timeLeft := time.Until(waitUntil).Round(time.Second)
			found := len(courses) - remaining
			PrintWaitingStatus(spin, attempt, found, len(courses), timeLeft.String(), checkTime)
			time.Sleep(100 * time.Millisecond)
			spin++
		}
	}

	PrintAllCoursesFound()
}
