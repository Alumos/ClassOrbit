package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"

	legacyxls "github.com/kardianos/xls"
	"github.com/xuri/excelize/v2"
)

var legacyExcelSignature = []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}

func readStudentExcelRows(file multipart.File) ([][]string, error) {
	signature := make([]byte, len(legacyExcelSignature))
	n, _ := io.ReadFull(file, signature)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if n == len(signature) && bytes.Equal(signature, legacyExcelSignature) {
		return readLegacyExcelRows(file)
	}

	book, err := excelize.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer book.Close()
	sheets := book.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("Excel 中没有工作表")
	}
	return book.GetRows(sheets[0])
}

func readLegacyExcelRows(file io.ReadSeeker) (rows [][]string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			rows = nil
			err = fmt.Errorf("读取旧版 Excel 失败: %v", recovered)
		}
	}()

	book, err := legacyxls.OpenReader(file, "utf-8")
	if err != nil {
		return nil, err
	}
	defer book.Close()
	if book.NumSheets() == 0 {
		return nil, fmt.Errorf("Excel 中没有工作表")
	}
	sheet, err := book.GetSheet(0)
	if err != nil {
		return nil, err
	}

	rows = make([][]string, 0, int(sheet.MaxRow)+1)
	for rowIndex := 0; rowIndex <= int(sheet.MaxRow); rowIndex++ {
		row := sheet.Row(rowIndex)
		if row == nil {
			rows = append(rows, nil)
			continue
		}
		values := make([]string, row.LastCol())
		for column := range values {
			values[column] = row.Col(column)
		}
		rows = append(rows, values)
	}
	return rows, nil
}
