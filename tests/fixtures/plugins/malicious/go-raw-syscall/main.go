package main

import "syscall"

func main() {
	// execve syscall = 59 on x86_64
	_, _, _ = syscall.Syscall(59, 0, 0, 0)
}
