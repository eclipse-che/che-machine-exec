//
// Copyright (c) 2026 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package validation

import (
	"fmt"
	"strings"
)

// ValidateNoShellMetacharacters checks if a string contains shell metacharacters
// that could be used for command injection attacks.
func ValidateNoShellMetacharacters(value, fieldName string) error {
	if value == "" {
		return nil // empty values are allowed
	}

	dangerousChars := []string{
		"$",   // variable expansion
		"`",   // command substitution
		";",   // command separator
		"|",   // pipe
		"&",   // background/and
		">",   // redirection
		"<",   // redirection
		"\n",  // newline
		"\r",  // carriage return
		"\"",  // double quote
		"'",   // single quote
		"\\",  // escape character
		"!",   // history expansion (bash)
		"*",   // glob expansion
		"?",   // glob single char wildcard
		"[",   // character class
		"]",   // character class
		"{",   // brace expansion
		"}",   // brace expansion
		"(",   // subshell / command substitution
		")",   // subshell / command substitution
	}

	for _, char := range dangerousChars {
		if strings.Contains(value, char) {
			return fmt.Errorf("%s contains forbidden character %q", fieldName, char)
		}
	}
	return nil
}
