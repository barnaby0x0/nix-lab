package loadgen

import (
	"fmt"
	"math/rand/v2"
)

var (
	levels     = []string{"INFO", "INFO", "INFO", "WARNING", "ERROR"}
	components = []string{"NETWORK", "FINOPS_ALRT", "FIREWALL", "SERVERLESS", "SEC", "IAM_SERVICE", "KERNEL", "DISK"}
	templates  = []func() string{
		func() string { return "WebAssembly sandbox isolated and killed: CPU execution timeout exceeded" },
		func() string {
			return fmt.Sprintf("Microservice 'payment-v2' reported high latency (%dms)", rand.IntN(900)+50)
		},
		func() string {
			return fmt.Sprintf("HNSW index build completed for 'knowledge-base' (%dms)", rand.IntN(1500)+100)
		},
		func() string { return "Forward proxy blocked connection to blacklisted category 'Gambling'" },
		func() string {
			return fmt.Sprintf("Process %d (node) sacrificed by Out-Of-Memory killer", rand.IntN(9000)+100)
		},
		func() string {
			return fmt.Sprintf("Connection closed prematurely by client: Ping timeout (%ds)", rand.IntN(600)+30)
		},
		func() string { return "Nginx buffer overflow prevented: client sent header line too long" },
		func() string {
			return fmt.Sprintf("Disk usage reached %d%% on partition /dev/sda1", rand.IntN(25)+75)
		},
		func() string {
			return fmt.Sprintf("SMTP relay rejected %d emails: Rate limit exceeded for sender", rand.IntN(1000)+10)
		},
	}
)
