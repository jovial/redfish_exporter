package collector

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/stmcginnis/gofish"
	redfish "github.com/stmcginnis/gofish/redfish"
)

// A ManagerCollector implements the prometheus.Collector.

type managerMetric struct {
	desc *prometheus.Desc
}

type logServiceMetric struct {
	desc *prometheus.Desc
}

//ManagerSubmanager is the manager subsystem
var (
	ManagerSubmanager = "manager"
	ManagerLabelNames = []string{"manager_id", "name", "model", "type"}
	managerMetrics    = map[string]managerMetric{
		"manager_state": {
			desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, ManagerSubmanager, "state"),
				"manager state,1(Enabled),2(Disabled),3(StandbyOffinline),4(StandbySpare),5(InTest),6(Starting),7(Absent),8(UnavailableOffline),9(Deferring),10(Quiesced),11(Updating)",
				ManagerLabelNames,
				nil,
			),
		},
		"manager_health_state": {
			desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, ManagerSubmanager, "health_state"),
				"manager health,1(OK),2(Warning),3(Critical)",
				ManagerLabelNames,
				nil,
			),
		},
		"manager_power_state": {
			desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, ManagerSubmanager, "power_state"),
				"manager power state",
				ManagerLabelNames,
				nil,
			),
		},
	}
)

var (
	logServiceLabelNames = []string{"name", "severity"}
	logServiceMetrics    = map[string]logServiceMetric{
		"entry_count": {
			desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "logservices", "entry_count"),
				"Number of log messages",
				logServiceLabelNames,
				nil,
			),
		},
	}
)

//ManagerCollector implemented prometheus.Collector
type ManagerCollector struct {
	redfishClient           *gofish.APIClient
	metrics                 map[string]managerMetric
	logServiceMetrics       map[string]logServiceMetric
	collectorScrapeStatus   *prometheus.GaugeVec
	collectorScrapeDuration *prometheus.SummaryVec
}

// NewManagerCollector returns a collector that collecting memory statistics
func NewManagerCollector(namespace string, redfishClient *gofish.APIClient) *ManagerCollector {
	return &ManagerCollector{
		redfishClient:     redfishClient,
		metrics:           managerMetrics,
		logServiceMetrics: logServiceMetrics,
		collectorScrapeStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "collector_scrape_status",
				Help:      "collector_scrape_status",
			},
			[]string{"collector"},
		),
	}
}

// Describe implemented prometheus.Collector
func (m *ManagerCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range m.metrics {
		ch <- metric.desc
	}
	m.collectorScrapeStatus.Describe(ch)

}

// Collect implemented prometheus.Collector
func (m *ManagerCollector) Collect(ch chan<- prometheus.Metric) {
	//get service
	service := m.redfishClient.Service

	// get a list of managers from service
	if managers, err := service.Managers(); err != nil {
		log.Infof("Errors Getting managers from service : %s", err)
	} else {

		for _, manager := range managers {
			// overall manager metrics

			ManagerID := manager.ID
			managerName := manager.Name
			managerModel := manager.Model
			managerType := fmt.Sprintf("%v", manager.ManagerType)
			managerPowerState := manager.PowerState
			managerState := manager.Status.State
			managerHealthState := manager.Status.Health

			ManagerLabelValues := []string{ManagerID, managerName, managerModel, managerType}

			if managerHealthStateValue, ok := parseCommonStatusHealth(managerHealthState); ok {
				ch <- prometheus.MustNewConstMetric(m.metrics["manager_health_state"].desc, prometheus.GaugeValue, managerHealthStateValue, ManagerLabelValues...)
			}
			if managerStateValue, ok := parseCommonStatusState(managerState); ok {
				ch <- prometheus.MustNewConstMetric(m.metrics["manager_state"].desc, prometheus.GaugeValue, managerStateValue, ManagerLabelValues...)
			}
			if managerPowerStateValue, ok := parseCommonPowerState(managerPowerState); ok {
				ch <- prometheus.MustNewConstMetric(m.metrics["manager_power_state"].desc, prometheus.GaugeValue, managerPowerStateValue, ManagerLabelValues...)

			}

			//TODO: move to logservice_collector.go
			if logServices, err := manager.LogServices(); err != nil {
				log.Infof("Errors LogServices from manager: %s", err)
			} else {
				for _, logService := range logServices {
					logServiceName := logService.Name
					if logEntries, err := logService.Entries(); err != nil {
						log.Infof("Errors LogEntries from Logservices: %s", err)
					} else {
						var entryMap = map[redfish.EventSeverity][]*redfish.LogEntry{}
						for _, entry := range logEntries {
							entryMap[entry.Severity] = append(entryMap[entry.Severity], entry)
						}
						for severity, element := range entryMap {
							logServiceLabelValues := []string{logServiceName, string(severity)}
							count := float64(len(element))
							ch <- prometheus.MustNewConstMetric(m.logServiceMetrics["entry_count"].desc, prometheus.GaugeValue, count, logServiceLabelValues...)
						}
					}
				}
			}

		}
		m.collectorScrapeStatus.WithLabelValues("manager").Set(float64(1))

	}

}
