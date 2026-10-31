package diagnose

import (
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTarget string
		wantOpts   Options
		wantErr    string
	}{
		{
			name:       "target only",
			args:       []string{"web"},
			wantTarget: "web",
			wantOpts:   Options{Lines: DefaultLines},
		},
		{
			name:       "namespace",
			args:       []string{"api-0", "--namespace", "prod"},
			wantTarget: "api-0",
			wantOpts:   Options{Lines: DefaultLines, Namespace: "prod"},
		},
		{
			name:       "short namespace",
			args:       []string{"api-0", "-N", "prod"},
			wantTarget: "api-0",
			wantOpts:   Options{Lines: DefaultLines, Namespace: "prod"},
		},
		{
			name:       "namespace equals",
			args:       []string{"api-0", "--namespace=prod"},
			wantTarget: "api-0",
			wantOpts:   Options{Lines: DefaultLines, Namespace: "prod"},
		},
		{
			name:       "context and lines",
			args:       []string{"api-0", "--context", "staging", "--lines", "200"},
			wantTarget: "api-0",
			wantOpts:   Options{Lines: 200, Context: "staging"},
		},
		{
			name:       "flags",
			args:       []string{"web", "--no-logs", "--no-events", "--container", "app"},
			wantTarget: "web",
			wantOpts:   Options{Lines: DefaultLines, NoLogs: true, NoEvents: true, Container: "app"},
		},
		{
			name:       "short container",
			args:       []string{"web", "-c", "app"},
			wantTarget: "web",
			wantOpts:   Options{Lines: DefaultLines, Container: "app"},
		},
		{
			name:       "kind",
			args:       []string{"web", "--kind", "docker"},
			wantTarget: "web",
			wantOpts:   Options{Lines: DefaultLines, Kind: "docker"},
		},
		{
			name:       "after separator",
			args:       []string{"--", "-n"},
			wantTarget: "-n",
			wantOpts:   Options{Lines: DefaultLines},
		},
		{name: "missing target", args: nil, wantErr: "必须提供"},
		{name: "two targets", args: []string{"a", "b"}, wantErr: "一次只能诊断一个"},
		{name: "bad kind", args: []string{"a", "--kind", "nomad"}, wantErr: "只支持 docker 或 k8s"},
		{name: "docker with namespace", args: []string{"a", "--kind", "docker", "-N", "prod"}, wantErr: "不支持 --namespace"},
		{name: "bad lines", args: []string{"a", "--lines", "0"}, wantErr: "正整数"},
		{name: "unknown option", args: []string{"a", "--wat"}, wantErr: "未知选项"},
		{name: "missing value", args: []string{"a", "--namespace"}, wantErr: "缺少参数"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, opts, err := ParseArgs(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target != tc.wantTarget {
				t.Errorf("target = %q, want %q", target, tc.wantTarget)
			}
			if opts != tc.wantOpts {
				t.Errorf("options = %+v, want %+v", opts, tc.wantOpts)
			}
		})
	}
}
