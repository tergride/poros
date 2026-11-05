// Package vmtest holds checks that change the system: a TUN, routes, a killed core. They run only in the test
// machine, where POROS_VM_TESTS=1 and POROS_TEST_CORE names the built PorosCore.exe, never on a computer someone
// works on.
package vmtest
