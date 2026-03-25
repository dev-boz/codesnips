package ansi

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseCSI(body string) (string, []int) {
	if body == "" {
		return "", nil
	}

	private := ""
	if strings.ContainsAny(string(body[0]), "?><=!") {
		private = string(body[0])
		body = body[1:]
	}
	if body == "" {
		return private, nil
	}

	parts := strings.Split(body, ";")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			values = append(values, 0)
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			value = 0
		}
		values = append(values, value)
	}
	return private, values
}

func ParamOr(params []int, index, fallback int) int {
	if index >= len(params) || params[index] == 0 {
		return fallback
	}
	return params[index]
}

func FormatCUP(row, col int) string {
	return fmt.Sprintf("\x1b[%d;%dH", row, col)
}

func FormatPrivateCSI(private string, params []int, final byte) string {
	parts := make([]string, 0, len(params))
	for _, param := range params {
		parts = append(parts, fmt.Sprintf("%d", param))
	}
	return fmt.Sprintf("\x1b[%s%s%c", private, strings.Join(parts, ";"), final)
}
