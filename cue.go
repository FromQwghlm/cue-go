// Package cue implement CUE-SHEET files parser.
// For CUE documentation see: http://digitalx.org/cue-sheet/syntax/
package cue

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// commandParser is the function for parsing one command.
type commandParser func(params []string, sheet *CueSheet) error

// commandParserDesctiptor describes command parser.
type commandParserDescriptor struct {
	// -1 -- zero or more parameters.
	paramsCount int
	parser      commandParser
}

// parsersMap used for commands and parser functions correspondence.
var parsersMap = map[string]commandParserDescriptor{
	"CATALOG":    {1, parseCatalog},
	"CDTEXTFILE": {1, parseCdTextFile},
	"FILE":       {2, parseFile},
	"FLAGS":      {-1, parseFlags},
	"INDEX":      {2, parseIndex},
	"ISRC":       {1, parseIsrc},
	"PERFORMER":  {1, parsePerformer},
	"POSTGAP":    {1, parsePostgap},
	"PREGAP":     {1, parsePregap},
	"REM":        {-1, parseRem},
	"SONGWRITER": {1, parseSongWriter},
	"TITLE":      {1, parseTitle},
	"TRACK":      {2, parseTrack},
}

// Parse parses cue-sheet data (file) and returns filled CueSheet struct.
func Parse(reader io.Reader) (sheet *CueSheet, err error) {
	sheet = new(CueSheet)

	rd := bufio.NewReader(reader)
	lineNumber := 0

	for buf, _, err := rd.ReadLine(); err != io.EOF; buf, _, err = rd.ReadLine() {
		if err != nil {
			return nil, err
		}

		line := strings.TrimSpace(string(buf))

		// Skip empty lines.
		if len(line) == 0 {
			continue
		}

		cmd, params, err := parseCommand(line)
		if err != nil {
			return nil, fmt.Errorf("Line %d. %s", err.Error())
		}

		lineNumber++

		parserDescriptor, ok := parsersMap[cmd]
		if !ok {
			return nil, fmt.Errorf("Line %d. Unknown command '%s'", lineNumber, cmd)
		}

		paramsExpected := parserDescriptor.paramsCount
		paramsRecieved := len(params)
		if paramsExpected != -1 && paramsExpected != paramsRecieved {
			return nil, fmt.Errorf("Line %d. Command %s: recieved %d parameters but %d expected",
				lineNumber, cmd, paramsRecieved, paramsExpected)
		}

		err = parserDescriptor.parser(params, sheet)
		if err != nil {
			return nil, fmt.Errorf("Line %d. Failed to parse %s command. %s", lineNumber, cmd, err.Error())
		}
	}

	return sheet, nil
}

// parseCatalog parsers CATALOG command.
func parseCatalog(params []string, sheet *CueSheet) error {
	num := params[0]

	matched, _ := regexp.MatchString("^[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]$", num)
	if !matched {
		return fmt.Errorf("%s is not valid catalog number", params)
	}

	sheet.Catalog = num

	return nil
}

// parseCdTextFile parsers CDTEXTFILE command.
func parseCdTextFile(params []string, sheet *CueSheet) error {
	sheet.CdTextFile = params[0]

	return nil
}

// parseFile parsers FILE command.
// params[0] -- fileName
// params[1] -- fileType
func parseFile(params []string, sheet *CueSheet) error {
	// Type parser function.
	parseFileType := func(t string) (fileType FileType, err error) {
		var types = map[string]FileType{
			"BINARY":   FileTypeBinary,
			"MOTOROLA": FileTypeMotorola,
			"AIFF":     FileTypeAiff,
			"WAVE":     FileTypeWave,
			"MP3":      FileTypeMp3,
		}

		fileType, ok := types[t]
		if !ok {
			err = fmt.Errorf("Unknown file type %s", t)
		}

		return
	}

	fileType, err := parseFileType(params[1])
	if err != nil {
		return err
	}

	file := *new(File)
	file.Name = params[0]
	file.Type = fileType

	sheet.Files = append(sheet.Files, file)

	return nil
}

// parseFlags parsers FLAGS command.
func parseFlags(params []string, sheet *CueSheet) error {
	flagParser := func(flag string) (trackFlag TrackFlag, err error) {
		var flags = map[string]TrackFlag{
			"DCP":  TrackFlagDcp,
			"4CH":  TrackFlag4ch,
			"PRE":  TrackFlagPre,
			"SCMS": TrackFlagScms,
		}

		trackFlag, ok := flags[flag]
		if !ok {
			err = fmt.Errorf("Unknown track flag %s", flag)
		}

		return
	}

	track := getCurrentTrack(sheet)
	if track == nil {
		return errors.New("TRACK command should appears before FLAGS command")
	}

	for _, flagStr := range params {
		flag, err := flagParser(flagStr)
		if err != nil {
			return err
		}
		track.Flags = append(track.Flags, flag)
	}

	return nil
}

// parseIndex parsers INDEX command.
func parseIndex(params []string, sheet *CueSheet) error {
	min, sec, frames, err := parseTime(params[1])
	if err != nil {
		return fmt.Errorf("Failed to parse index start time. %s", err.Error())
	}

	number, err := strconv.Atoi(params[0])
	if err != nil {
		return fmt.Errorf("Failed to parse index number. %s", err.Error())
	}

	// All index numbers must be between 0 and 99 inclusive.
	if number < 0 || number > 99 {
		return errors.New("Index number should be in 0..99 interval")
	}

	track := getCurrentTrack(sheet)
	if track == nil {
		return fmt.Errorf("TRACK command should appears before INDEX command")
	}

	// The first index of a file must start at 00:00:00.
	if getFileLastIndex(getCurrentFile(sheet)) == nil {
		if min+sec+frames != 0 {
			return errors.New("First track index must start at 00:00:00")
		}
	}

	// This is the first track index?
	if len(track.Indexes) == 0 {
		// The first index must be 0 or 1.
		if number >= 2 {
			return errors.New("First track index should has 0 or 1 inxed number")
		}
	} else {
		// All other indexes being sequential to the first one.
		numberExpected := track.Indexes[len(track.Indexes)-1].Number + 1
		if numberExpected != number {
			return fmt.Errorf("Expected %d index number but %d recieved", numberExpected, number)
		}
	}

	index := Index{Number: number, Time: Time{min, sec, frames}}
	track.Indexes = append(track.Indexes, index)

	return nil
}

// parseIsrc parsers ISRC command.
func parseIsrc(params []string, sheet *CueSheet) error {
	isrc := params[0]

	track := getCurrentTrack(sheet)
	if track == nil {
		return errors.New("TRACK command should appears before ISRC command")
	}

	if len(track.Indexes) != 0 {
		return errors.New("ISRC command must be specified before INDEX command")
	}

	re := "^[0-9a-zA-z][0-9a-zA-z][0-9a-zA-z][0-9a-zA-z][0-9a-zA-z]" +
		"[0-9][0-9][0-9][0-9][0-9][0-9][0-9]$"
	matched, _ := regexp.MatchString(re, isrc)
	if !matched {
		return fmt.Errorf("%s is not valid ISRC number", isrc)
	}

	track.Isrc = isrc

	return nil
}

// parsePerformer parsers PERFORMER command.
func parsePerformer(params []string, sheet *CueSheet) error {
	// Limit this field length up to 80 characters.
	performer := stringTruncate(params[0], 80)
	track := getCurrentTrack(sheet)

	if track == nil {
		// Performer command for the CD disk.
		sheet.Performer = performer
	} else {
		// Performer command for track.
		track.Performer = performer
	}

	return nil
}

// parsePostgap parsers POSTGAP command.
func parsePostgap(params []string, sheet *CueSheet) error {
	track := getCurrentTrack(sheet)
	if track == nil {
		return errors.New("POSTGAP command must appear after a TRACK command")
	}

	min, sec, frames, err := parseTime(params[0])
	if err != nil {
		return fmt.Errorf("Failed to parse postgap time. %s", err.Error())
	}

	track.Postgap = Time{min, sec, frames}

	return nil
}

// parsePregap parsers PREGAP command.
func parsePregap(params []string, sheet *CueSheet) error {
	track := getCurrentTrack(sheet)
	if track == nil {
		return errors.New("PREGAP command must appear after a TRACK command")
	}

	if len(track.Indexes) != 0 {
		return errors.New("PREGAP command must appear before any INDEX command")
	}

	min, sec, frames, err := parseTime(params[0])
	if err != nil {
		return fmt.Errorf("Failed to parse pregap time. %s", err.Error())
	}

	track.Pregap = Time{min, sec, frames}

	return nil
}

// parseRem parsers REM command.
func parseRem(params []string, sheet *CueSheet) error {
	sheet.Comments = append(sheet.Comments, strings.Join(params, " "))

	return nil
}

// parseSongWriter parsers SONGWRITER command.
func parseSongWriter(params []string, sheet *CueSheet) error {
	// Limit this field length up to 80 characters.
	songwriter := stringTruncate(params[0], 80)
	track := getCurrentTrack(sheet)

	if track == nil {
		sheet.Songwriter = songwriter
	} else {
		track.Songwriter = songwriter
	}

	return nil
}

// parseTitle parsers TITLE command.
func parseTitle(params []string, sheet *CueSheet) error {
	// Limit this field length up to 80 characters.
	title := stringTruncate(params[0], 80)
	track := getCurrentTrack(sheet)

	if track == nil {
		// Title for the CD disk.
		sheet.Title = title
	} else {
		// Title command for track.
		track.Title = title
	}

	return nil
}

// parseTrack parses TRACK command.
func parseTrack(params []string, sheet *CueSheet) error {
	// TRACK command should be after FILE command.
	if len(sheet.Files) == 0 {
		return fmt.Errorf("Unexpected TRACK command. FILE command expected first.")
	}

	numberStr := params[0]
	dataTypeStr := params[1]

	// Type parser function.
	parseDataType := func(t string) (dataType TrackDataType, err error) {
		var types = map[string]TrackDataType{
			"AUDIO":      DataTypeAudio,
			"CDG":        DataTypeCdg,
			"MODE1/2048": DataTypeMode1_2048,
			"MODE1/2352": DataTypeMode1_2352,
			"MODE2/2336": DataTypeMode2_2336,
			"MODE2/2352": DataTypeMode2_2352,
			"CDI/2336":   DataTypeCdi_2336,
			"CDI/2352":   DataTypeCdi_2352,
		}

		dataType, ok := types[t]
		if !ok {
			err = fmt.Errorf("Unknown track datatype %s", t)
		}

		return
	}

	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return fmt.Errorf("Failed to parse track number parameter. %s", err.Error())
	}
	if number < 1 {
		return fmt.Errorf("Failed to parse track number parameter. Value should be in 1..99 range.")
	}

	dataType, err := parseDataType(dataTypeStr)
	if err != nil {
		return err
	}

	track := new(Track)
	track.Number = number
	track.DataType = dataType

	file := &sheet.Files[len(sheet.Files)-1]

	// But all track numbers after the first must be sequential.
	if len(file.Tracks) > 0 {
		if file.Tracks[len(file.Tracks)-1].Number != number-1 {
			return fmt.Errorf("Expected track number %d, but %d recieved.",
				number-1, number)
		}
	}

	file.Tracks = append(file.Tracks, *track)

	return nil
}

// getCurrentFile returns file object started with the last FILE command.
// Returns nil if there is no any File objects.
func getCurrentFile(sheet *CueSheet) *File {
	if len(sheet.Files) == 0 {
		return nil
	}

	return &sheet.Files[len(sheet.Files)-1]
}

// getCurrentTrack returns current track object, which was started with last TRACK command.
// Returns nil if there is no any Track object avaliable.
func getCurrentTrack(sheet *CueSheet) *Track {
	file := getCurrentFile(sheet)
	if file == nil {
		return nil
	}

	if len(file.Tracks) == 0 {
		return nil
	}

	return &file.Tracks[len(file.Tracks)-1]
}

// getFileLastIndex returns last index for the given file.
// Returns nil if file has no any indexes.
func getFileLastIndex(file *File) *Index {
	for i := len(file.Tracks) - 1; i >= 0; i-- {
		track := &file.Tracks[i]

		for j := len(track.Indexes) - 1; j >= 0; j-- {
			return &track.Indexes[j]
		}
	}

	return nil
}
