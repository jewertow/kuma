package api_server_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	api_server_config "github.com/kumahq/kuma/pkg/config/api-server"
	"github.com/kumahq/kuma/pkg/metrics"
	"github.com/kumahq/kuma/pkg/plugins/resources/memory"
)

var _ = Describe("Config WS", func() {

	It("should return the config", func() {
		// given
		cfg := api_server_config.DefaultApiServerConfig()

		// setup
		resourceStore := memory.NewStore()
		metrics, err := metrics.NewMetrics("Standalone")
		Expect(err).ToNot(HaveOccurred())
		apiServer := createTestApiServer(resourceStore, cfg, true, metrics)

		stop := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			err := apiServer.Start(stop)
			Expect(err).ToNot(HaveOccurred())
		}()
		port := strings.Split(apiServer.Address(), ":")[1]

		// wait for the server
		Eventually(func() error {
			_, err := http.Get(fmt.Sprintf("http://localhost:%s/config", port))
			return err
		}, "3s").ShouldNot(HaveOccurred())

		// when
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/config", port))
		Expect(err).ToNot(HaveOccurred())

		// then
		body, err := ioutil.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		json := fmt.Sprintf(`
		{
		  "apiServer": {
			"auth": {
			  "allowFromLocalhost": true,
			  "clientCertsDir": "../../test/certs/client"
			},
			"corsAllowedDomains": [
			  ".*"
			],
			"http": {
			  "enabled": true,
			  "interface": "0.0.0.0",
			  "port": %s
			},
			"https": {
			  "enabled": true,
			  "interface": "0.0.0.0",
			  "port": %d,
			  "tlsCertFile": "../../test/certs/server-cert.pem",
			  "tlsKeyFile": "../../test/certs/server-key.pem"
			},
			"readOnly": false
		  },
		  "bootstrapServer": {
			"apiVersion": "v3",
			"params": {
			  "adminAccessLogPath": "/dev/null",
			  "adminAddress": "127.0.0.1",
			  "adminPort": 0,
			  "xdsConnectTimeout": "1s",
			  "xdsHost": "",
			  "xdsPort": 0
			}
		  },
		  "defaults": {
			"skipMeshCreation": false
		  },
		  "diagnostics": {
			"debugEndpoints": false,
			"serverPort": 5680
		  },
		  "dnsServer": {
			"CIDR": "240.0.0.0/4",
			"domain": "mesh",
			"port": 5653
		  },
		  "dpServer": {
			"auth": {
			  "type": ""
			},
			"hds": {
			  "checkDefaults": {
				"healthyThreshold": 1,
				"interval": "1s",
				"noTrafficInterval": "1s",
				"timeout": "2s",
				"unhealthyThreshold": 1
			  },
			  "enabled": true,
			  "interval": "5s",
			  "refreshInterval": "10s"
			},
			"port": 5678,
			"tlsCertFile": "",
			"tlsKeyFile": ""
		  },
		  "environment": "universal",
		  "general": {
			"dnsCacheTTL": "10s",
			"tlsCertFile": "",
			"tlsKeyFile": "",
			"workDir": ""
		  },
		  "guiServer": {
			"apiServerUrl": ""
		  },
		  "metrics": {
			"dataplane": {
			  "enabled": true,
			  "subscriptionLimit": 10
			},
			"mesh": {
			  "maxResyncTimeout": "20s",
			  "minResyncTimeout": "1s"
			},
			"zone": {
			  "enabled": true,
			  "subscriptionLimit": 10
			}
		  },
		  "mode": "standalone",
		  "monitoringAssignmentServer": {
			"apiVersions": [
			  "v1alpha1",
			  "v1"
			],
			"assignmentRefreshInterval": "1s",
			"defaultFetchTimeout": "30s",
			"grpcPort": 0,
			"port": 5676
		  },
		  "multizone": {
			"global": {
			  "kds": {
				"grpcPort": 5685,
				"refreshInterval": "1s",
				"tlsCertFile": "",
				"tlsKeyFile": "",
				"zoneInsightFlushInterval": "10s"
			  },
			  "pollTimeout": "500ms"
			},
			"zone": {
			  "kds": {
				"refreshInterval": "1s",
				"rootCaFile": ""
			  }
			}
		  },
		  "reports": {
			"enabled": true
		  },
		  "runtime": {
			"kubernetes": {
			  "admissionServer": {
				"address": "",
				"certDir": "",
				"port": 5443
			  },
			  "controlPlaneServiceName": "kuma-control-plane",
			  "injector": {
				"caCertFile": "",
				"builtinDNS": {
                  "port": 15053
                },
                "cniEnabled": false,
				"exceptions": {
				  "labels": {
					"openshift.io/build.name": "*",
					"openshift.io/deployer-pod-for.name": "*"
				  }
				},
				"initContainer": {
				  "image": "kuma/kuma-init:latest"
				},
				"sidecarContainer": {
				  "adminPort": 9901,
				  "drainTime": "30s",
				  "envVars": {},
				  "gid": 5678,
				  "image": "kuma/kuma-dp:latest",
				  "livenessProbe": {
					"failureThreshold": 12,
					"initialDelaySeconds": 60,
					"periodSeconds": 5,
					"timeoutSeconds": 3
				  },
				  "readinessProbe": {
					"failureThreshold": 12,
					"initialDelaySeconds": 1,
					"periodSeconds": 5,
					"successThreshold": 1,
					"timeoutSeconds": 3
				  },
				  "redirectPortInbound": 15006,
				  "redirectPortInboundV6": 15010,
                  "redirectPortOutbound": 15001,
                  "resources": {
                    "limits": {
                      "cpu": "1000m",
                      "memory": "512Mi"
                    },
                    "requests": {
                      "cpu": "50m",
                      "memory": "64Mi"
                    }
                  },
                  "uid": 5678
                },
                "sidecarTraffic": {
                  "excludeInboundPorts": [],
                  "excludeOutboundPorts": []
                },
                "virtualProbesEnabled": true,
                "virtualProbesPort": 9000,
                "exceptions": {
                  "labels": {
                    "openshift.io/build.name": "*",
                    "openshift.io/deployer-pod-for.name": "*"
                  }
                },
                "caCertFile": ""
              },
              "marshalingCacheExpirationTime": "5m0s"
            },
            "universal": {
              "dataplaneCleanupAge": "72h0m0s"
            }
          },
          "sdsServer": {
            "dataplaneConfigurationRefreshInterval": "1s"
          },
          "dpServer": {
            "port": 5678,
            "tlsCertFile": "",
            "tlsKeyFile": "",
            "auth": {
              "type": ""
            },
            "hds": {
              "checkDefaults": {
                "healthyThreshold": 1,
                "interval": "1s",
                "noTrafficInterval": "1s",
                "timeout": "2s",
                "unhealthyThreshold": 1
              },
              "enabled": true,
              "interval": "5s",
              "refreshInterval": "10s"
            }
          },
          "store": {
            "kubernetes": {
              "systemNamespace": "kuma-system"
            },
            "postgres": {
              "connectionTimeout": 5,
              "dbName": "kuma",
              "host": "127.0.0.1",
              "maxIdleConnections": 0,
              "maxOpenConnections": 0,
              "password": "*****",
              "port": 15432,
              "maxReconnectInterval": "1m0s",
              "minReconnectInterval": "10s",
              "tls": {
                "certPath": "",
                "keyPath": "",
                "mode": "disable",
                "caPath": ""
              },
              "user": "kuma"
            },
            "cache": {
              "enabled": true,
              "expirationTime": "1s"
            },
            "upsert": {
              "conflictRetryBaseBackoff": "100ms",
              "conflictRetryMaxTimes": 5
            },
            "type": "memory"
          },
          "xdsServer": {
            "dataplaneConfigurationRefreshInterval": "1s",
            "dataplaneStatusFlushInterval": "10s",
            "nackBackoff": "5s"
          },
          "diagnostics": {
            "serverPort": 5680,
            "debugEndpoints": false
          }
        }
		`, port, cfg.HTTPS.Port)
		// when
		Expect(body).To(MatchJSON(json))
	})
})
