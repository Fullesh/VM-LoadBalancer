package collector

import (
	"VM-LoadBalancer/model"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type VMClient struct {
	baseURL    string
	httpClient *http.Client
}

type sample struct {
	Labels map[string]string
	Value  float64
}

type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]any            `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// мини-помощник создания гипервизора если его нет
func getOrCreateHost(hosts map[string]*model.Hypervisor, name string) *model.Hypervisor {
	if hv, exists := hosts[name]; exists {
		return hv
	}
	hv := &model.Hypervisor{Name: name}
	hosts[name] = hv
	return hv
}

// мини-помощник создания VM если её нет
func getOrCreateVM(vms map[string]*model.VirtualMachine, name, hostname string) *model.VirtualMachine {
	if vm, exists := vms[name]; exists {
		if vm.HypervisorName == "" && hostname != "" {
			vm.HypervisorName = hostname
		}
		return vm
	}
	vm := &model.VirtualMachine{Name: name, HypervisorName: hostname}
	vms[name] = vm
	return vm
}

func (c *VMClient) query(ctx context.Context, promQL string) ([]sample, error) {
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", c.baseURL, url.QueryEscape(promQL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("неожиданный HTTP-статус: %d", resp.StatusCode)
	}

	var parsed promResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("статус ответа VictoriaMetrics: %s", parsed.Status)
	}
	samples := make([]sample, 0, len(parsed.Data.Result))
	for _, item := range parsed.Data.Result {
		strVal, ok := item.Value[1].(string)
		if !ok {
			return nil, fmt.Errorf("не удалось преобразовать значение метрики в строку")
		}
		val, err := strconv.ParseFloat(strVal, 64)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample{Labels: item.Metric, Value: val})
	}
	return samples, nil
}

func NewVMClient(baseURL string) *VMClient {
	baseURL = strings.TrimRight(baseURL, "/")
	httpClient := &http.Client{Timeout: 5 * time.Second}
	return &VMClient{baseURL: baseURL, httpClient: httpClient}
}

func (c *VMClient) Collect(ctx context.Context) ([]model.Hypervisor, []model.VirtualMachine, error) {
	hosts := make(map[string]*model.Hypervisor)
	vms := make(map[string]*model.VirtualMachine)

	vmQueries := []struct {
		promQL string
		apply  func(vm *model.VirtualMachine, val float64)
	}{
		{"libvirt_domain_info_virtual_cpus", func(vm *model.VirtualMachine, v float64) { vm.CpuCapacityCores = v }},
		{"libvirt_domain_info_memory_usage_bytes", func(vm *model.VirtualMachine, v float64) { vm.MemoryUsageBytes = v }},
		{"libvirt_domain_info_maximum_memory_bytes", func(vm *model.VirtualMachine, v float64) { vm.MemoryCapacityBytes = v }},
		{"max by (domain) (rate(libvirt_domain_info_cpu_time_seconds_total[30s]))", func(vm *model.VirtualMachine, v float64) { vm.CpuUsageCores = v }},
	}
	hvQueries := []struct {
		promQL string
		apply  func(hv *model.Hypervisor, val float64)
	}{
		{"node_cpu_cores_total", func(hv *model.Hypervisor, v float64) { hv.CpuCapacityCores = v }},
		{"node_memory_total_bytes", func(hv *model.Hypervisor, v float64) { hv.MemoryCapacityBytes = v }},
	}

	for _, query := range vmQueries {
		samples, err := c.query(ctx, query.promQL)
		if err != nil {
			return nil, nil, fmt.Errorf("ошибка выполнения запроса %q: %w", query.promQL, err)
		}
		for _, sample := range samples {
			vmName := sample.Labels["domain"]
			hostName := sample.Labels["host"]
			if vmName == "" {
				continue
			}
			vm := getOrCreateVM(vms, vmName, hostName)
			query.apply(vm, sample.Value)
		}

	}
	for _, query := range hvQueries {
		samples, err := c.query(ctx, query.promQL)
		if err != nil {
			return nil, nil, fmt.Errorf("ошибка выполнения запроса %q: %w", query.promQL, err)
		}
		for _, sample := range samples {
			hostName := sample.Labels["host"]
			if hostName == "" {
				continue
			}
			hv := getOrCreateHost(hosts, hostName)
			query.apply(hv, sample.Value)
		}
	}

	for _, vm := range vms {
		if hv, exists := hosts[vm.HypervisorName]; exists {
			hv.MachinesInside = append(hv.MachinesInside, vm.Name)
			hv.CpuUsageCores += vm.CpuUsageCores
			hv.MemoryUsageBytes += vm.MemoryUsageBytes
		}
	}
	resHosts := make([]model.Hypervisor, 0, len(hosts))
	for _, hv := range hosts {
		resHosts = append(resHosts, *hv)
	}
	resVMs := make([]model.VirtualMachine, 0, len(vms))
	for _, vm := range vms {
		resVMs = append(resVMs, *vm)
	}

	return resHosts, resVMs, nil
}
