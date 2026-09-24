package generation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type coverageUnit struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type coveragePlan struct {
	Task       string         `json:"required_task"`
	Units      []coverageUnit `json:"required_units"`
	Ordered    bool           `json:"preserve_order"`
	Unverified bool           `json:"complete_coverage_unverifiable"`
}

var (
	exactTask     = regexp.MustCompile(`(?i)\b(verbatim|word[- ]for[- ]word|exact (text|wording)|recite|recitation|memori[sz]e (the |this )?(poem|verse|passage|text))\b`)
	completeTask  = regexp.MustCompile(`(?i)(^(please )?(learn |memori[sz]e |teach me |quiz me (on )?|list )?(all |every |the complete |the entire )|\b(complete (set|list)|entire (set|list|sequence)|exhaustive|every (item|member|entry)|all (items|members|entries)|in (the )?(original|given) order)\b)`)
	incompleteSet = regexp.MustCompile(`(?i)\b(sample|examples?|partial|incomplete|non[- ]exhaustive|including|such as|etc)\b|\.{3}|…`)
	listLine      = regexp.MustCompile(`^\s*(?:[-*•]\s+|[0-9]{1,3}[.)]\s+|[A-Za-z][.)]\s+)(.+)$`)
	mappingLine   = regexp.MustCompile(`^\s*[\p{L}\p{N}][\p{L}\p{N} ]{0,40}\s*(?:→|=>|:| — | – | - )\s*\S.+$`)
)

func planTask(text, kind string) (coveragePlan, error) {
	plan := coveragePlan{Task: "infer", Units: []coverageUnit{}}
	if strings.TrimSpace(text) == "" || len(text) > maxSourceBytes || !utf8.ValidString(text) || hasUnsafeControl(text) {
		return plan, errors.New("Saved input is empty, invalid text, or exceeds 128 KiB. Save a smaller complete excerpt; nothing is silently truncated.")
	}
	if kind != "source" && kind != "topic" {
		return plan, errors.New("Saved input has unsupported provenance. Capture it again as a topic or a supplied excerpt.")
	}
	firstLine, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if exactTask.MatchString(firstLine) {
		plan.Task, plan.Ordered = "exact_text", true
		body := text
		if colon := strings.Index(firstLine, ":"); colon >= 0 {
			// Retain the exact supplied text after the instruction delimiter.
			offset := strings.Index(text, firstLine)
			body = text[offset+colon+1:]
		} else if _, rest, found := strings.Cut(text, "\n"); found {
			body = rest
		} else {
			plan.Unverified = true
			return plan, nil
		}
		if kind != "source" || strings.TrimSpace(body) == "" {
			plan.Unverified = true
			return plan, nil
		}
		for _, line := range strings.Split(body, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			// A leading space after an inline instruction is a separator, not
			// silently edited poem text. Multiline indentation is retained.
			if len(plan.Units) == 0 && !strings.HasPrefix(body, "\n") && strings.Contains(firstLine, ":") {
				line = strings.TrimLeft(line, " \t")
			}
			if len(line) > 1024 {
				return plan, errors.New("An exact-text unit exceeds the 1024-byte answer limit. Split at an intentional line or sentence boundary; exact text will not be paraphrased or sampled.")
			}
			plan.Units = append(plan.Units, coverageUnit{ID: fmt.Sprintf("u%d", len(plan.Units)+1), Text: line})
		}
	} else {
		if completeTask.MatchString(firstLine) {
			plan.Task, plan.Unverified = "complete_set", true
		}
		var units []coverageUnit
		nonempty := 0
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			nonempty++
			unit := ""
			if match := listLine.FindStringSubmatch(line); match != nil {
				unit = match[1]
			} else if mappingLine.MatchString(line) {
				unit = trimmed
			}
			if unit != "" {
				units = append(units, coverageUnit{ID: fmt.Sprintf("u%d", len(units)+1), Text: unit})
			}
		}
		if len(units) >= 2 && len(units)*2 >= nonempty {
			plan.Task, plan.Ordered, plan.Unverified = "complete_set", true, kind != "source"
			plan.Units = units
		}
		if plan.Task == "complete_set" && incompleteSet.MatchString(text) {
			plan.Unverified = true
		}
	}
	if len(plan.Units) > maxQuizzes {
		return plan, errors.New("This complete-set or exact-text task needs more than 60 quizzes. Split it into explicitly bounded sections; generation will not quietly sample it.")
	}
	return plan, nil
}

func hasUnsafeControl(text string) bool {
	for _, char := range text {
		if unicode.IsControl(char) && char != '\n' && char != '\r' && char != '\t' {
			return true
		}
	}
	return false
}
