package main

import (
	"os"
	"os/exec"
	"strings"
)

// CheckGit returns nil if directory contains a valid git working tree, error otherwise
func CheckGit(directory string) error {
	current, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(current)
	err = os.Chdir(directory)
	if err != nil {
		return err
	}
	_, err = exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return err
	}
	return nil
}

// PushRemote performs git steps on supplied file in directory: stage, commit, push
func PushRemote(directory string, fileToCommit string, comments []string) (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	defer os.Chdir(current)
	err = os.Chdir(directory)
	if err != nil {
		return "Cannot set directory", err
	}
	_, err = exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return "Status error:", err
	}
	_, err = exec.Command("git", "add", fileToCommit).Output()
	if err != nil {
		return "Staging error", err
	}
	_, err = exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return "Status staging error:", err
	}
	var sb strings.Builder
	sb.WriteString("-m\"")
	for i, c := range comments {
		sb.WriteString(c)
		if i < len(comments)-1 {
			sb.WriteByte(',')
		}
	}
	sb.WriteByte('"')
	comment := sb.String()
	_, err = exec.Command("git", "commit", comment).Output()
	if err != nil {
		return "Commit error:", err
	}
	_, err = exec.Command("git", "push").Output()
	if err != nil {
		return "Push error:", err
	}
	return "Success", nil
}
