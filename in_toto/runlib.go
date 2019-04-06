package in_toto

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	//"github.com/golang/tools/internal/fastwalk"

	
)

/*
RecordArtifact reads and hashes the contents of the file at the passed path
using sha256 and returns a map in the following format:
  {
    "<path>": {
      "sha256": <hex representation of hash>
    }
  }
If reading the file fails, the first return value is nil and the second return
value is the error.
*/
func RecordArtifact(path string) (map[string]interface{}, error) {
	// Read file from passed path
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Create its sha 256 hash (currently we only support sha256 here)
	hashed := sha256.Sum256(content)

	// Return it in a format that is conformant with link metadata artifacts
	return map[string]interface{}{
		"sha256": fmt.Sprintf("%x", hashed),
	}, nil
}

/*
RecordArtifacts walks through the passed slice of paths, traversing
subdirectories, and calls RecordArtifact for each file.  It returns a map in
the following format:
  {
    "<path>": {
      "sha256": <hex representation of hash>
    },
    "<path>": {
      "sha256": <hex representation of hash>
    },
    ...
  }
If recording an artifact fails the first return value is nil and the second
return value is the error.
*/
func RecordArtifacts(paths []string) (map[string]interface{}, error) {
	artifacts := make(map[string]interface{})
	// NOTE: Walk cannot follow symlinks
	for _, path := range paths {
		err := filepath.Walk(path,
			func(path string, info os.FileInfo, err error) error {
				// Abort if Walk function has a problem, e.g. path does not exist)
				if err != nil {
					return err
				}
				// Don't hash directories
				if info.IsDir() {
					return nil
				}
				//Code to verify for symlinks	
				if info.Mode() & os.ModeSymlink != 0{
					//fmt.Println("symlink found", path)
					//a, _ := filepath.EvalSymlinks(path)
					sym_path, sym_err := os.Readlink(path) 
					if sym_err != nil {
						return sym_err
					}
					//fmt.Println("symlink path", sym_path,sym_err)
				//	fmt.Println(filepath.walkSymlinks(path))	
					//path_till_now := []string{"tmpdir/"}	
					//new_path := append(path_till_now, sym_path)		
					//fmt.Println("new path" + "appended",new_path)		
					recursed_artifacts, recursed_err := RecordArtifacts([]string{sym_path})
					if recursed_err != nil {
						return recursed_err
					}
					for key, value := range recursed_artifacts{
						//fmt.Println("Recursed key", key, "Recursed value", value)
						artifacts[key] = value
					}
					return nil
				}
				artifact, err := RecordArtifact(path)

				// Abort if artifact can't be recorded, e.g. due to file permissions
				if err != nil {
					return err
				}
				artifacts[path] = artifact
				return nil
			})

		if err != nil {
			return nil, err
		}
	}
	//Looking at the artifacts
	//fmt.Println("Here are the Artifacts *******")
	//for key, value := range artifacts{
	//	fmt.Println("key", key, "value", value)	
	//}
	//fmt.Println("END OF ONE RECUSION *****************")
	return artifacts, nil
}

/*
WaitErrToExitCode converts an error returned by Cmd.wait() to an exit code.  It
returns -1 if no exit code can be inferred.
*/
func WaitErrToExitCode(err error) int {
	// If there's no exit code, we return -1
	retVal := -1

	// See https://stackoverflow.com/questions/10385551/get-exit-code-go
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0
			// This works on both Unix and Windows. Although package
			// syscall is generally platform dependent, WaitStatus is
			// defined for both Unix and Windows and in both cases has
			// an ExitStatus() method with the same signature.
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				retVal = status.ExitStatus()
			}
		}
	} else {
		retVal = 0
	}

	return retVal
}

/*
RunCommand executes the passed command in a subprocess.  The first element of
cmdArgs is used as executable and the rest as command arguments.  It captures
and returns stdout, stderr and exit code.  The format of the returned map is:
  {
    "return-value": <exit code>,
    "stdout": "<standard output>",
    "stderr": "<standard error>"
  }
If the command cannot be executed or no pipes for stdout or stderr can be
created the first return value is nil and the second return value is the error.
NOTE: Since stdout and stderr are captured, they cannot be seen during the
command execution.
*/
func RunCommand(cmdArgs []string) (map[string]interface{}, error) {

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// TODO: duplicate stdout, stderr
	stdout, _ := ioutil.ReadAll(stdoutPipe)
	stderr, _ := ioutil.ReadAll(stderrPipe)

	retVal := WaitErrToExitCode(cmd.Wait())

	return map[string]interface{}{
		"return-value": retVal,
		"stdout":       stdout,
		"stderr":       stderr,
	}, nil
}

/*
InTotoRun executes commands, e.g. for software supply chain steps or
inspections of an in-toto layout, and creates and returns corresponding link
metadata.  Link metadata contains recorded products at the passed productPaths
and materials at the passed materialPaths.  The returned link is wrapped in a
Metablock object.  If command execution or artifact recording fails the first
return value is an empty Metablock and the second return value is the error.
NOTE: Currently InTotoRun cannot be used to sign Link metadata.
*/
func InTotoRun(name string, materialPaths []string, productPaths []string,
	cmdArgs []string) (Metablock, error) {
	var linkMb Metablock
	materials, err := RecordArtifacts(materialPaths)
	if err != nil {
		return linkMb, err
	}

	byProducts, err := RunCommand(cmdArgs)
	if err != nil {
		return linkMb, err
	}

	products, err := RecordArtifacts(productPaths)
	if err != nil {
		return linkMb, err
	}

	linkMb.Signatures = []Signature{}
	linkMb.Signed = Link{
		Type:        "link",
		Name:        name,
		Materials:   materials,
		Products:    products,
		ByProducts:  byProducts,
		Command:     cmdArgs,
		Environment: map[string]interface{}{},
	}

	return linkMb, nil
}
