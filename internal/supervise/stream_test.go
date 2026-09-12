package supervise

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"omastack/internal/model"
	"omastack/internal/paths"
	"omastack/internal/redact"
)

func TestOversizedLogLineCannotBlockService(t *testing.T) {
	dir := t.TempDir()
	svc := model.Service{ID: "22222222-2222-4222-8222-222222222222", Command: model.CommandSpec{Executable: "/usr/bin/head", Arguments: []string{"-c", "2097152", "/dev/zero"}}, WorkingDirectory: dir, StopSignal: "SIGTERM", GracefulStopSeconds: 1}
	record := RuntimeRecord{ServiceID: svc.ID}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	code, _, err := runOnce(ctx, paths.Paths{RuntimeServicesDir: dir}, svc, &record, time.Now())
	if ctx.Err() != nil || code != 0 || err != nil {
		t.Fatalf("finite output blocked: code=%d err=%v ctx=%v", code, err, ctx.Err())
	}
}

func TestOversizedLinesDrainWithoutSecretFragments(t *testing.T) {
	secret := "PRIVATE-TOKEN"
	input := strings.Repeat("x", (1<<20)-4) + secret + "\n" + strings.Repeat(secret, 100000) + "\nnormal " + secret + "\nlast"
	var output bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	copyStream(&wg, &output, "stdout", strings.NewReader(input), []string{secret})
	wg.Wait()
	got := output.String()
	if strings.Contains(got, "PRIV") || !strings.Contains(got, "normal "+redact.Mask+"\nstdout\tlast\n") {
		t.Fatalf("unsafe or incomplete output: %.200s", got)
	}
	if strings.Count(got, "oversized log line omitted") != 2 {
		t.Fatalf("missing omission markers: %.200s", got)
	}
}
