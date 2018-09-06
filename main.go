package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

const (
	DOC_INDENT = "\t\t\t\t     "

	LONG_DOC = `A not-so-distant cousin of xargs, map can be used to functionally compose tasks.`
	EXAMPLES = `	Delete all the build directories in a folder:
		find . -name build -type d | map d 'echo Deleting $d && rm -rf $d'

	ssh onto multiple machines and print the hostname followed by disk usage:
		map -f hosts -e 'ssh $1 df /' | awk '/\//{print $1, $6}'`
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	cmd := NewCmd()
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func NewCmd() *cobra.Command {
	flags := new(MapFlags)

	var cmd = &cobra.Command{
		Use:     `map [flags] [VAR...] COMMAND`,
		Long:    LONG_DOC,
		Example: EXAMPLES,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Run(flags, args)
		},
		Args: cobra.ArbitraryArgs,
	}

	cmd.Flags().IntVarP(&flags.NumJobs, "workers", "j", 1, "Same as -P/--parallelism")
	cmd.Flags().IntVarP(&flags.NumJobs, "parallelism", "P", 1, "Maximum number of workers used to map the input")
	cmd.Flags().StringSliceVarP(&flags.InputFiles, "files", "f", nil, "Files from which to read input")
	cmd.Flags().BoolVarP(&flags.NoTrimWhitespace, "no-trim-whitespace", "s", false,
		"Don't trim leading and trailing whitespace characters")
	cmd.Flags().StringVarP(&flags.FieldDelimiter, "field-delimiter", "d", `\s`,
		"Field delimiter for multi param executions. Defaults to \"\\s+\" (supports regex)")
	cmd.Flags().StringVarP(&flags.LineDelimiter, "line-delimiter", "l", "\n",
		"Delimiter for groups of fields. The nth field will contain everything between\n"+
			DOC_INDENT+"the n-1th delimiter up to the line delimiter. Please use pipe tricks like\n"+
			DOC_INDENT+"'tr ; \"\\n\"' to deterministically separate sets of fields instead. Use at\n"+
			DOC_INDENT+"your own risk!")
	cmd.Flags().BoolVarP(&flags.EchoInput, "echo", "e", false,
		"Similar to prepending \"echo -n \\\"$@\\\" && \" to your function. Input to be echoed back \n"+
			DOC_INDENT+"before the output of the command is printed. Does not add newlines.")
	cmd.Flags().StringVarP(&flags.OutputDelimiter, "output-delimiter", "o", " ",
		"Can only be used in conjunction with the --echo flag, this specifies the delimiter used to separate the\n"+
			DOC_INDENT+"input and output. Defaults to \" \"")

	return cmd
}

type MapFlags struct {
	NumJobs          int
	InputFiles       []string
	NoTrimWhitespace bool
	FieldDelimiter   string
	LineDelimiter    string
	EchoInput        bool
	OutputDelimiter  string
}

func Run(flags *MapFlags, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("No function given!")
	}

	for _, arg := range args {
		if len(arg) == 0 {
			return fmt.Errorf("No empty string allowed as VAR or COMMAND")
		}
	}

	if len(flags.InputFiles) == 0 {
		stat, err := os.Stdin.Stat()
		if err != nil {
			return err
		}
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return fmt.Errorf("No stdin and no files given.")
		}
	}

	mapper := Mapper{
		workers:         flags.NumJobs,
		echoInput:       flags.EchoInput,
		outputDelimiter: flags.OutputDelimiter,
	}

	fieldSplitter := regexp.MustCompile(flags.FieldDelimiter + "+")
	if len(args) == 1 {
		mapper.generateCmd = NumberedVars(args[0], fieldSplitter)
	} else {
		mapper.generateCmd = NamedVars(args[len(args)-1], args[:len(args)-1], fieldSplitter)
	}

	var lineSplitter = bufio.ScanLines
	if flags.LineDelimiter != "\n" {
		lineSplitter = customLineSplitter(flags.LineDelimiter)
	}

	inputChannel := make(chan string, 1<<4)
	go ReadFiles(flags.InputFiles, lineSplitter, !flags.NoTrimWhitespace, inputChannel)

	for out := range mapper.Map(inputChannel) {
		fmt.Println(out)
	}

	return nil
}

// copied from bufio.ScanLines, except without removing the \r character
func customLineSplitter(lineDelimiter string) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := bytes.IndexAny(data, lineDelimiter); i >= 0 {
			return i + 1, data[0:i], nil
		}

		if atEOF {
			return len(data), data, nil
		}

		return 0, nil, nil
	}
}

func ReadFiles(files []string, lineSplitter bufio.SplitFunc, trimWhitespace bool, inputChannel chan string) {
	defer close(inputChannel)

	if len(files) == 0 {
		ReadAllLines(os.Stdin, lineSplitter, trimWhitespace, inputChannel)
	} else {
		for _, filename := range files {
			f, err := os.Open(filename)
			if err != nil {
				log.Fatalf("Could not open %s: %v\n", filename, err)
			}
			ReadAllLines(f, lineSplitter, trimWhitespace, inputChannel)
			f.Close()
		}
	}
}

func ReadAllLines(f *os.File, lineSplitter bufio.SplitFunc, trimWhitespace bool, inputChannel chan string) {
	scanner := bufio.NewScanner(bufio.NewReader(f))
	scanner.Split(lineSplitter)
	for scanner.Scan() {
		line := scanner.Text()
		if trimWhitespace {
			line = strings.TrimSpace(line)
		}
		inputChannel <- line
	}
}

func NamedVars(command string, varNames []string, fieldSplitter *regexp.Regexp) cmdGenerator {
	return func(input string) *exec.Cmd {
		process := exec.Command("/bin/bash", "-c", command)
		env := os.Environ()
		for i, v := range fieldSplitter.Split(input, len(varNames)) {
			env = append(env, fmt.Sprintf("%s=%s", varNames[i], v))
		}
		process.Env = env
		return process
	}
}

func NumberedVars(command string, fieldSplitter *regexp.Regexp) cmdGenerator {
	return func(input string) *exec.Cmd {
		params := []string{"-c", command, ""}
		params = append(params, fieldSplitter.Split(input, -1)...)
		return exec.Command("/bin/bash", params...)
	}
}
