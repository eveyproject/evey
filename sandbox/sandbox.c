#define _GNU_SOURCE
#include <linux/seccomp.h>
#include <linux/filter.h>
#include <linux/audit.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <stddef.h>
#include <errno.h>

#ifndef __NR_execveat
#define __NR_execveat 322
#endif
#ifndef __NR_process_vm_readv
#define __NR_process_vm_readv 310
#endif
#ifndef __NR_process_vm_writev
#define __NR_process_vm_writev 311
#endif
#ifndef __NR_setns
#define __NR_setns 308
#endif
#ifndef __NR_finit_module
#define __NR_finit_module 313
#endif
#ifndef __NR_perf_event_open
#define __NR_perf_event_open 298
#endif
#ifndef __NR_clock_adjtime
#define __NR_clock_adjtime 305
#endif
#ifndef __NR_kexec_load
#define __NR_kexec_load 246
#endif
#ifndef __NR_kexec_file_load
#define __NR_kexec_file_load 320
#endif
#ifndef __NR_bpf
#define __NR_bpf 321
#endif
#ifndef __NR_add_key
#define __NR_add_key 248
#endif
#ifndef __NR_request_key
#define __NR_request_key 249
#endif
#ifndef __NR_keyctl
#define __NR_keyctl 250
#endif

/*
 * Denylist — syscalls with no legitimate use in a plugin context, grouped by
 * category: exec / process debug / mount / namespace / kernel modules /
 * bpf+perf / hw I/O / swap / system control / kexec / setuid family /
 * capabilities / keyring.
 *
 * Pattern: all BPF_JEQ jumps forward to the final RET_ERRNO; false branch
 * falls through.  RET_ALLOW sits just before RET_ERRNO.
 * Each JEQ at 0-based index i (range 1..N) uses jt = N+1-i to reach RET_ERRNO.
 * N = 38 JEQ instructions → RET_ALLOW at [39], RET_ERRNO at [40].
 */
static struct sock_filter filter[] = {
    /* [0] */ BPF_STMT(BPF_LD | BPF_W | BPF_ABS, offsetof(struct seccomp_data, nr)),

    /* exec */
    /* [1]  */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_execve,           38, 0),
    /* [2]  */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_execveat,         37, 0),

    /* process control / debug */
    /* [3]  */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_ptrace,           36, 0),
    /* [4]  */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_process_vm_readv, 35, 0),
    /* [5]  */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_process_vm_writev,34, 0),

    /* filesystem mount */
    /* [6]  */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_mount,            33, 0),
    /* [7]  */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_umount2,          32, 0),
    /* [8]  */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_pivot_root,       31, 0),
    /* [9]  */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_chroot,           30, 0),

    /* namespaces */
    /* [10] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_unshare,          29, 0),
    /* [11] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_setns,            28, 0),

    /* kernel modules */
    /* [12] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_init_module,      27, 0),
    /* [13] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_finit_module,     26, 0),
    /* [14] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_delete_module,    25, 0),

    /* BPF / perf */
    /* [15] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_bpf,              24, 0),
    /* [16] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_perf_event_open,  23, 0),

    /* hardware I/O */
    /* [17] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_ioperm,           22, 0),
    /* [18] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_iopl,             21, 0),

    /* swap / memory mgmt */
    /* [19] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_swapon,           20, 0),
    /* [20] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_swapoff,          19, 0),

    /* system control */
    /* [21] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_syslog,           18, 0),
    /* [22] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_reboot,           17, 0),
    /* [23] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_settimeofday,     16, 0),
    /* [24] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_clock_settime,    15, 0),
    /* [25] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_clock_adjtime,    14, 0),
    /* [26] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_adjtimex,         13, 0),

    /* kexec */
    /* [27] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_kexec_load,       12, 0),
    /* [28] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_kexec_file_load,  11, 0),

    /* setuid family (plugin runs as UID 1001 — escalation forbidden) */
    /* [29] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_setuid,           10, 0),
    /* [30] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_setgid,            9, 0),
    /* [31] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_setreuid,          8, 0),
    /* [32] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_setregid,          7, 0),
    /* [33] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_setresuid,         6, 0),
    /* [34] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_setresgid,         5, 0),

    /* capabilities */
    /* [35] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_capset,            4, 0),

    /* keyring */
    /* [36] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_add_key,           3, 0),
    /* [37] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_request_key,       2, 0),
    /* [38] */ BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_keyctl,            1, 0),

    /* [39] */ BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ALLOW),
    /* [40] */ BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ERRNO | (EPERM & SECCOMP_RET_DATA)),
};

static struct sock_fprog prog = {
    .len = sizeof(filter) / sizeof(filter[0]),
    .filter = filter,
};

__attribute__((constructor))
static void sandbox_init(void) {
    prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0);
    prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, &prog);
}
