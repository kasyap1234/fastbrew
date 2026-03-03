package cmd

import (
	"fastbrew/internal/brew"
	"fastbrew/internal/services"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var serviceScope string

var servicesCmd = &cobra.Command{
	Use:   "services",
	Short: "Manage Homebrew services",
	Long:  "Start, stop, restart, and list Homebrew-installed services",
}

var servicesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all services",
	Run: func(cmd *cobra.Command, args []string) {
		if daemonClient, daemonErr := getDaemonClientForRead(); daemonClient != nil {
			svcs, err := daemonClient.ServicesList(serviceScope)
			if err == nil {
				printServices(svcs)
				return
			}
			warnDaemonFallback("services list", err)
		} else if daemonErr != nil {
			warnDaemonFallback("services list", daemonErr)
		}

		mgr := getServiceManager()
		svcs, err := mgr.ListServices()
		if err != nil {
			fmt.Printf("Error listing services: %v\n", err)
			os.Exit(1)
		}

		printServices(svcs)
	},
}

var servicesStartCmd = &cobra.Command{
	Use:   "start <service>",
	Short: "Start a service",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mgr := getServiceManager()
		if err := mgr.Start(args[0]); err != nil {
			fmt.Printf("Error starting %s: %v\n", args[0], err)
			os.Exit(1)
		}
		notifyDaemonInvalidation(brew.EventServiceChanged)
		fmt.Printf("✅ Started %s\n", args[0])
	},
}

var servicesStopCmd = &cobra.Command{
	Use:   "stop <service>",
	Short: "Stop a service",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mgr := getServiceManager()
		if err := mgr.Stop(args[0]); err != nil {
			fmt.Printf("Error stopping %s: %v\n", args[0], err)
			os.Exit(1)
		}
		notifyDaemonInvalidation(brew.EventServiceChanged)
		fmt.Printf("✅ Stopped %s\n", args[0])
	},
}

var servicesRestartCmd = &cobra.Command{
	Use:   "restart <service>",
	Short: "Restart a service",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mgr := getServiceManager()
		if err := mgr.Restart(args[0]); err != nil {
			fmt.Printf("Error restarting %s: %v\n", args[0], err)
			os.Exit(1)
		}
		notifyDaemonInvalidation(brew.EventServiceChanged)
		fmt.Printf("✅ Restarted %s\n", args[0])
	},
}

func getServiceManager() services.ServiceManager {
	scope := services.ServiceScope(serviceScope)
	if scope == "" {
		return services.NewServiceManager()
	}
	mgr, err := services.NewServiceManagerWithScope(scope)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	return mgr
}

func init() {
	servicesListCmd.Flags().StringVar(&serviceScope, "scope", "", "Scope to use (user, system, all)")
	servicesStartCmd.Flags().StringVar(&serviceScope, "scope", "", "Scope to use (user, system, all)")
	servicesStopCmd.Flags().StringVar(&serviceScope, "scope", "", "Scope to use (user, system, all)")
	servicesRestartCmd.Flags().StringVar(&serviceScope, "scope", "", "Scope to use (user, system, all)")

	servicesCmd.AddCommand(servicesListCmd)
	servicesCmd.AddCommand(servicesStartCmd)
	servicesCmd.AddCommand(servicesStopCmd)
	servicesCmd.AddCommand(servicesRestartCmd)
	rootCmd.AddCommand(servicesCmd)
}

func printServices(svcs []services.Service) {
	if len(svcs) == 0 {
		fmt.Println("No services found.")
		return
	}

	fmt.Printf("%-30s %-10s %-10s\n", "NAME", "STATUS", "PID")
	fmt.Println("--------------------------------------------------")
	for _, svc := range svcs {
		pid := "-"
		if svc.Pid > 0 {
			pid = fmt.Sprintf("%d", svc.Pid)
		}
		fmt.Printf("%-30s %-10s %-10s\n", svc.Name, svc.Status, pid)
	}
}
