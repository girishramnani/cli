// Copyright © 2019 The Tekton Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package taskrun

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/cli/pkg/formatted"
	"github.com/tektoncd/cli/pkg/printer"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cliopts "k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	emptyMsg = "No taskruns found"
)

func listCommand(p cli.Params) *cobra.Command {
	f := cliopts.NewPrintFlags("list")
	eg := `
# List all TaskRuns of Task name 'foo'
tkn taskrun list  foo -n bar

# List all taskruns in a namespaces 'foo'
tkn pr list -n foo \n",
`

	c := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "Lists taskruns in a namespace",
		Example: eg,
		RunE: func(cmd *cobra.Command, args []string) error {
			var task string

			if len(args) > 0 {
				task = args[0]
			}

			trs, err := list(p, task)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to list taskruns from %s namespace \n", p.Namespace())
				return err
			}

			output, err := cmd.LocalFlags().GetString("output")
			if err != nil {
				fmt.Fprint(os.Stderr, "Error: output option not set properly \n")
				return err
			}

			if output != "" {
				return printer.PrintObject(cmd.OutOrStdout(), trs, f)
			}
			stream := &cli.Stream{
				Out: cmd.OutOrStdout(),
				Err: cmd.OutOrStderr(),
			}
			err = printFormatted(stream, trs, p.Time())
			if err != nil {
				fmt.Fprint(os.Stderr, "Failed to print Taskruns \n")
				return err
			}
			return nil
		},
	}

	f.AddFlags(c)

	return c
}

func list(p cli.Params, task string) (*v1alpha1.TaskRunList, error) {
	cs, err := p.Clients()
	if err != nil {
		return nil, err
	}

	options := v1.ListOptions{}
	if task != "" {
		options = v1.ListOptions{
			LabelSelector: fmt.Sprintf("tekton.dev/task=%s", task),
		}
	}

	trc := cs.Tekton.TektonV1alpha1().TaskRuns(p.Namespace())
	trs, err := trc.List(options)
	if err != nil {
		return nil, err
	}

	if len(trs.Items) != 0 {
		sort.Sort(byStartTime(trs.Items))
	}

	// NOTE: this is required for -o json|yaml to work properly since
	// tektoncd go client fails to set these; probably a bug
	trs.GetObjectKind().SetGroupVersionKind(
		schema.GroupVersionKind{
			Version: "tekton.dev/v1alpha1",
			Kind:    "TaskRunList",
		})

	return trs, nil
}

func printFormatted(s *cli.Stream, trs *v1alpha1.TaskRunList, c clockwork.Clock) error {
	if len(trs.Items) == 0 {
		fmt.Fprintln(s.Err, emptyMsg)
		return nil
	}

	w := tabwriter.NewWriter(s.Out, 0, 5, 3, ' ', tabwriter.TabIndent)
	fmt.Fprintln(w, "NAME\tSTARTED\tDURATION\tSTATUS\t")
	for _, tr := range trs.Items {
		if len(tr.Status.Conditions) == 0 {
			tr.Status.InitializeConditions()
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t\n",
			tr.Name,
			formatted.Age(*tr.Status.StartTime, c),
			formatted.Duration(tr.Status.StartTime, tr.Status.CompletionTime),
			formatted.Condition(tr.Status.Conditions[0]),
		)
	}
	return w.Flush()
}

type byStartTime []v1alpha1.TaskRun

func (s byStartTime) Len() int           { return len(s) }
func (s byStartTime) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byStartTime) Less(i, j int) bool { return s[j].Status.StartTime.Before(s[i].Status.StartTime) }
