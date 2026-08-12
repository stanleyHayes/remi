package finance

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// renderTextPDF emits a deliberately small, deterministic PDF using only the
// built-in Helvetica fonts. It keeps financial artifacts reproducible without
// depending on a platform font renderer or mutable HTML-to-PDF service.
func renderTextPDF(title string, lines []string) []byte {
	const linesPerPage = 45
	if len(lines) == 0 {
		lines = []string{"No entries in this period."}
	}
	pageCount := (len(lines) + linesPerPage - 1) / linesPerPage
	objects := []string{"", ""} // catalog and pages tree are filled last.
	fontObject := len(objects) + 1
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	boldFontObject := len(objects) + 1
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>")
	pageObjects := make([]int, 0, pageCount)
	for page := 0; page < pageCount; page++ {
		start := page * linesPerPage
		end := start + linesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		var stream strings.Builder
		stream.WriteString("BT\n/F2 18 Tf\n54 790 Td\n(" + pdfEscape(title) + ") Tj\n")
		stream.WriteString("/F1 9 Tf\n0 -25 Td\n")
		for _, line := range lines[start:end] {
			stream.WriteString("(" + pdfEscape(line) + ") Tj\n0 -15 Td\n")
		}
		stream.WriteString("ET\n")
		content := stream.String()
		contentObject := len(objects) + 1
		objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content))
		pageObject := len(objects) + 1
		pageObjects = append(pageObjects, pageObject)
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 %d 0 R /F2 %d 0 R >> >> /Contents %d 0 R >>", fontObject, boldFontObject, contentObject))
	}
	kids := make([]string, len(pageObjects))
	for index, object := range pageObjects {
		kids[index] = strconv.Itoa(object) + " 0 R"
	}
	objects[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids))

	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n%REMI\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index < len(offsets); index++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}

func pdfEscape(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return ' '
		}
		return r
	}, value)
	return strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(value)
}
