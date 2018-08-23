//
// Copyright (c) 2012-2018 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package model

import (
	"bufio"
	"bytes"
	"log"
	"unicode/utf8"
)

// read byte array as Unicode code points (rune in go)
func normalizeBuffer(normalizedBuf *bytes.Buffer, buf []byte, n int) (int, error) {
	bufferBytes := normalizedBuf.Bytes()
	runeReader := bufio.NewReader(bytes.NewReader(append(bufferBytes[:], buf[:n]...)))
	normalizedBuf.Reset()
	i := 0
	for i < n {
		char, charLen, err := runeReader.ReadRune()
		if err != nil {
			return i, err
		}
		if char == utf8.RuneError {
			if err := runeReader.UnreadRune(); err != nil {
				log.Print(err)
			}
			return i, nil
		}
		i += charLen
		if _, err := normalizedBuf.WriteRune(char); err != nil {
			return i, err
		}
	}
	return i, nil
}
