package get

import (
	"context"
	mesh_proto "github.com/Kong/kuma/api/mesh/v1alpha1"
	util_proto "github.com/Kong/kuma/pkg/util/proto"
	"io"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/Kong/kuma/app/kumactl/pkg/output"
	"github.com/Kong/kuma/app/kumactl/pkg/output/printers"
	"github.com/Kong/kuma/app/kumactl/pkg/output/table"
	"github.com/Kong/kuma/pkg/core/resources/apis/mesh"
	mesh_core "github.com/Kong/kuma/pkg/core/resources/apis/mesh"
	rest_types "github.com/Kong/kuma/pkg/core/resources/model/rest"
)

type getDataplanesContext struct {
	*listContext

	args struct {
		compact	bool
		tags    map[string]string
		gateway bool
	}
}

func newGetDataplanesCmd(pctx *listContext) *cobra.Command {
	ctx := getDataplanesContext{
		listContext: pctx,
	}
	cmd := &cobra.Command{
		Use:   "dataplanes",
		Short: "Show Dataplanes",
		Long:  `Show Dataplanes.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := pctx.CurrentDataplaneOverviewClient()
			if err != nil {
				return errors.Wrap(err, "failed to create a dataplane client")
			}
			overviews, err := client.List(context.Background(), pctx.CurrentMesh(), ctx.args.tags, ctx.args.gateway)
			if err != nil {
				return err
			}

			switch format := output.Format(pctx.getContext.args.outputFormat); format {
			case output.TableFormat:
				return printDataplaneOverviews(pctx.Now(), overviews, cmd.OutOrStdout(), ctx.args.compact)
			default:
				printer, err := printers.NewGenericPrinter(format)
				if err != nil {
					return err
				}
				return printer.Print(rest_types.From.ResourceList(overviews), cmd.OutOrStdout())
			}
		},
	}
	cmd.PersistentFlags().BoolVarP(&ctx.args.compact, "compact", "", false, "print only columns MESH, NAME, TAGS and AGE")
	cmd.PersistentFlags().StringToStringVarP(&ctx.args.tags, "tag", "", map[string]string{}, "filter by tag in format of key=value. You can provide many tags")
	cmd.PersistentFlags().BoolVarP(&ctx.args.gateway, "gateway", "", false, "filter gateway dataplanes")
	return cmd
}

func printDataplaneOverviews(now time.Time, dataplaneInsights *mesh.DataplaneOverviewResourceList, out io.Writer, compact bool) error {
	if compact {
		return printCompactDataplaneOverviews(now, dataplaneInsights, out)
	} else {
		return printFullDataplaneOverviews(now, dataplaneInsights, out)
	}
}

// TODO: Dodac kolumnę age
func printFullDataplaneOverviews(now time.Time, dataplaneInsights *mesh_core.DataplaneOverviewResourceList, out io.Writer) error {
	data := printers.Table{
		Headers: []string{"MESH", "NAME", "TAGS", "STATUS", "LAST CONNECTED AGO", "LAST UPDATED AGO", "TOTAL UPDATES", "TOTAL ERRORS", "CERT REGENERATED AGO", "CERT EXPIRATION", "CERT REGENERATIONS"},
		NextRow: func() func() []string {
			i := 0
			return func() []string {
				defer func() { i++ }()
				if len(dataplaneInsights.Items) <= i {
					return nil
				}
				meta := dataplaneInsights.Items[i].Meta
				dataplane := dataplaneInsights.Items[i].Spec.Dataplane
				dataplaneInsight := dataplaneInsights.Items[i].Spec.DataplaneInsight

				lastSubscription, lastConnected := dataplaneInsight.GetLatestSubscription()
				totalResponsesSent := dataplaneInsight.Sum(func(s *mesh_proto.DiscoverySubscription) uint64 {
					return s.GetStatus().GetTotal().GetResponsesSent()
				})
				totalResponsesRejected := dataplaneInsight.Sum(func(s *mesh_proto.DiscoverySubscription) uint64 {
					return s.GetStatus().GetTotal().GetResponsesRejected()
				})
				onlineStatus := "Offline"
				if dataplaneInsight.IsOnline() {
					onlineStatus = "Online"
				}
				lastUpdated := util_proto.MustTimestampFromProto(lastSubscription.GetStatus().GetLastUpdateTime())

				var certExpiration *time.Time
				if dataplaneInsight.GetMTLS().GetCertificateExpirationTime() != nil {
					certExpiration = util_proto.MustTimestampFromProto(dataplaneInsight.GetMTLS().GetCertificateExpirationTime())
				}
				var lastCertGeneration *time.Time
				if dataplaneInsight.GetMTLS().GetLastCertificateRegeneration() != nil {
					lastCertGeneration = util_proto.MustTimestampFromProto(dataplaneInsight.GetMTLS().GetLastCertificateRegeneration())
				}
				dataplaneInsight.GetMTLS().GetCertificateExpirationTime()
				certRegenerations := strconv.Itoa(int(dataplaneInsight.GetMTLS().GetCertificateRegenerations()))

				return []string{
					meta.GetMesh(),                       // MESH
					meta.GetName(),                       // NAME,
					dataplane.Tags().String(),            // TAGS
					onlineStatus,                         // STATUS
					table.Ago(lastConnected, now),        // LAST CONNECTED AGO
					table.Ago(lastUpdated, now),          // LAST UPDATED AGO
					table.Number(totalResponsesSent),     // TOTAL UPDATES
					table.Number(totalResponsesRejected), // TOTAL ERRORS
					table.Ago(lastCertGeneration, now),   // CERT REGENERATED AGO
					table.Date(certExpiration),           // CERT EXPIRATION
					certRegenerations,                    // CERT REGENERATIONS
				}
			}
		}(),
	}
	return printers.NewTablePrinter().Print(data, out)
}

func printCompactDataplaneOverviews(rootTime time.Time, dataplanes *mesh.DataplaneOverviewResourceList, out io.Writer) error {
	data := printers.Table{
		Headers: []string{"MESH", "NAME", "TAGS", "AGE"},
		NextRow: func() func() []string {
			i := 0
			return func() []string {
				defer func() { i++ }()
				if len(dataplanes.Items) <= i {
					return nil
				}
				dataplane := dataplanes.Items[i]

				return []string{
					dataplane.Meta.GetMesh(),                                        // MESH
					dataplane.Meta.GetName(),                                        // NAME,
					dataplane.Spec.Dataplane.Tags().String(),                        // TAGS
					table.TimeSince(dataplane.Meta.GetModificationTime(), rootTime), // AGE
				}
			}
		}(),
		Footer: table.PaginationFooter(dataplanes),
	}
	return printers.NewTablePrinter().Print(data, out)
}

