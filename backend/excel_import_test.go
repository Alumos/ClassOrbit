package main

import (
	"bytes"
	"encoding/base64"
	"testing"
)

type memoryMultipartFile struct {
	*bytes.Reader
}

func (*memoryMultipartFile) Close() error {
	return nil
}

func TestParseLegacyExcel(t *testing.T) {
	contents, err := base64.StdEncoding.DecodeString(legacyStudentsXLSBase64)
	if err != nil {
		t.Fatal(err)
	}
	file := &memoryMultipartFile{Reader: bytes.NewReader(contents)}
	students, err := parseExcel(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 2 ||
		students[0].StudentNo != "2026001" || students[0].Name != "陈同学" ||
		students[1].StudentNo != "2026002" || students[1].Name != "王同学" {
		t.Fatalf("parsed students = %+v", students)
	}
}

const legacyStudentsXLSBase64 = "" +
	"0M8R4KGxGuEAAAAAAAAAAAAAAAAAAAAAPgADAP7/CQAGAAAAAAAAAAAAAAABAAAAAgAAAAAAAAAAEAAAAQAAAAEAAAD+////AAAA" +
	"AAAAAAD/////////////////////////////////////////////////////////////////////////////////////////////" +
	"////////////////////////////////////////////////////////////////////////////////////////////////////" +
	"////////////////////////////////////////////////////////////////////////////////////////////////////" +
	"////////////////////////////////////////////////////////////////////////////////////////////////////" +
	"////////////////////////////////////////////////////////////////////////////////////////////////////" +
	"///////////////////////////////////////////////////////////////////////////////////9/////v////7///8E" +
	"AAAABQAAAP7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+////" +
	"/v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v//" +
	"//7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7/" +
	"///+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+" +
	"/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+////" +
	"/v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v//" +
	"//7////+/////v////7////+/////v////7////+/////v////7////+/////v////7///8CAAAAAwAAAAQAAAAFAAAABgAAAAcA" +
	"AAAIAAAACQAAAAoAAAALAAAADAAAAA0AAAAOAAAADwAAABAAAAARAAAAEgAAABMAAAD+/////v////7////+/////v////7////+" +
	"/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+////" +
	"/v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v//" +
	"//7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7/" +
	"///+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+" +
	"/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+////" +
	"/v////7////+/////v////7////+/////v////7////+////UgBvAG8AdAAgAEUAbgB0AHIAeQAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABYABQH//////////wEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"AAADAAAAAAUAAAAAAAABAFMAaAAzADMAdABKADUAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"AAAAEgACAf////8CAAAA/////wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAFcAbwByAGsA" +
	"YgBvAG8AawAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAASAAIB////////////////AAAA" +
	"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAALMEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD///////////////8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA3MjYyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"AAAAAAAAAAAAAAAACQgQAAAGBQBics0HCcABAAYHAADhAAIAsATBAAIAAADiAAAAXABwAAcAAFNoMzN0SlMAAAAAAAAAAAAAAAAA" +
	"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"AAAAAAAAAAAAAAAAAABCAAIAsARhAQIAAADAAQAAPQECAAEAnAACABEAGQACAAAAEgACAAAAEwACAAAArwECAAAAvAECAAAAPQAS" +
	"AAAAAABgcsBEOAAAAAAAAQD0AUAAAgAAAI0AAgAAACIAAgAAAA4AAgABALcBAgAAANoAAgAAADEAGgDwAAAAAACQAQAAAAAAAAUB" +
	"QQByAGkAYQBsAB4ENQA4ABgAASIACk5IUy8AC05IUyAAIgBoAGgAIgBCZiIAbQBtACIABlIiAHMAcwAiANJ5IAAiAOAAFAAAAAAA" +
	"9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8A" +
	"AAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAA" +
	"AAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAA" +
	"AAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAA" +
	"AAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAAAOAAFAAAAAAA9P8AAAAAAAAAAAAAAAAA" +
	"AOAAFAAAAAAAAAAAAAAAAAAAAAAAAAAAAGABAgAAAIUADAApAwAAAAACAQ1UVVOMAAQAAQABAPwACAAAAAAAAAAAAAoAAAAJCBAA" +
	"AAYQAGJyzQcJwAEABgcAAA0AAgABAAwAAgBkAA8AAgABABEAAgAAABAACAD8qfHSTWJQP18AAgABACoAAgAAACsAAgAAAIIAAgAB" +
	"AIAACAAAAAAAAAAAAIMAAgAAAIQAAgAAAAACDgAAAAAAAwAAAAAAAwAAAAQCDQAAAAAAEAACAAFmW/dTBAINAAAAAQAQAAIAAdNZ" +
	"DVQEAg0AAAACABAAAgABB1nobAQCFwABAAAAEAAHAAEyADAAMgA2ADAAMAAxAAQCDwABAAEAEAADAAFIlgxUZlsEAgkAAQACABAA" +
	"AAABBAIXAAIAAAAQAAcAATIAMAAyADYAMAAwADIABAIPAAIAAQAQAAMAAYtzDFRmWwQCCQACAAIAEAAAAAE+AhIAtgYAAAAAQAAA" +
	"AAAAAAAAAAAAugEHAAIAAQ1UVVNnCBMAZwgAAAAAAAAAAAAAAwABAAAAAGgIJwBoCAAAAAAAAAAAAAADAAAAAAAAAQAEAAAAAAAA" +
	"AAIAAAACAAQAAAAKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
