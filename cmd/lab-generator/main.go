package main

import (
	"VM-LoadBalancer/model"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"
)

func seedCluster(fc *FakeCluster) error {
	const GiB = 1024 * 1024 * 1024
	hosts := []string{"hv-01", "hv-02", "hv-03"}
	for _, name := range hosts {
		if _, err := fc.CreateHypervisor(name, 16, 64*GiB); err != nil {
			return err
		}
	}
	vms := []model.VirtualMachine{
		// Перегружаем hv-01
		{Name: "vm-db-1", HypervisorName: "hv-01", CpuUsageCores: 6, CpuCapacityCores: 8, MemoryUsageBytes: 24 * GiB, MemoryCapacityBytes: 32 * GiB},
		{Name: "vm-api-1", HypervisorName: "hv-01", CpuUsageCores: 4, CpuCapacityCores: 4, MemoryUsageBytes: 12 * GiB, MemoryCapacityBytes: 16 * GiB},
		{Name: "vm-cache-1", HypervisorName: "hv-01", CpuUsageCores: 3, CpuCapacityCores: 4, MemoryUsageBytes: 10 * GiB, MemoryCapacityBytes: 16 * GiB},
		// Средняя нагрузка на hv-02
		{Name: "vm-web-1", HypervisorName: "hv-02", CpuUsageCores: 3, CpuCapacityCores: 4, MemoryUsageBytes: 12 * GiB, MemoryCapacityBytes: 16 * GiB},
		// Почти пустой hv-03
		{Name: "vm-worker-1", HypervisorName: "hv-03", CpuUsageCores: 1, CpuCapacityCores: 2, MemoryUsageBytes: 4 * GiB, MemoryCapacityBytes: 8 * GiB},
	}
	for _, vm := range vms {
		if _, err := fc.CreateVM(vm); err != nil {
			return err
		}
	}
	return nil
}

type SimVM struct {
	model.VirtualMachine
	CpuTimeSecondsTotal float64
}

type FakeCluster struct {
	mu              sync.RWMutex
	Hypervisors     map[string]*model.Hypervisor
	VirtualMachines map[string]*SimVM
}

func (fc *FakeCluster) RunSimulation(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fc.mu.Lock()
			for _, vm := range fc.VirtualMachines {
				vm.CpuTimeSecondsTotal += vm.CpuUsageCores

			}
			fc.mu.Unlock()
		}
	}
}

// Метод создания виртуальной машины
func (fc *FakeCluster) CreateVM(vm model.VirtualMachine) (*SimVM, error) {
	if vm.Name == "" {
		return nil, fmt.Errorf("Имя VM не может быть пустым")
	}
	if vm.CpuUsageCores > vm.CpuCapacityCores || vm.MemoryUsageBytes > vm.MemoryCapacityBytes {
		return nil, fmt.Errorf("Загрузка не может быть выше ресурса")
	}
	if vm.HypervisorName == "" {
		return nil, fmt.Errorf("Должно быть задано имя гипервизора")
	}
	if vm.CpuCapacityCores <= 0 || vm.MemoryCapacityBytes <= 0 || vm.CpuUsageCores <= 0 || vm.MemoryUsageBytes <= 0 {
		return nil, fmt.Errorf("Ресурсы и Usage должны быть положительными и больше нуля")
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	hv, exists := fc.Hypervisors[vm.HypervisorName]
	if !exists {
		return nil, fmt.Errorf("Гипервизор %q не найден", vm.HypervisorName)
	}
	if _, exists := fc.VirtualMachines[vm.Name]; exists {
		return nil, fmt.Errorf("VM %q уже существует", vm.Name)
	}
	if hv.MemoryUsageBytes+vm.MemoryUsageBytes > hv.MemoryCapacityBytes {
		return nil, fmt.Errorf("Недостаточно RAM на гипервизоре %q", vm.HypervisorName)
	}

	simVM := &SimVM{VirtualMachine: vm}
	fc.VirtualMachines[vm.Name] = simVM
	hv.MachinesInside = append(hv.MachinesInside, vm.Name)
	hv.CpuUsageCores += vm.CpuUsageCores
	hv.MemoryUsageBytes += vm.MemoryUsageBytes

	return simVM, nil
}

// Метод получения метрик
func (fc *FakeCluster) GetData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	for _, hv := range fc.Hypervisors {
		fmt.Fprintf(w, "node_cpu_cores_total{host=\"%s\"} %.0f\n", hv.Name, hv.CpuCapacityCores)
		fmt.Fprintf(w, "node_memory_total_bytes{host=\"%s\"} %.0f\n", hv.Name, hv.MemoryCapacityBytes)
	}
	for _, vm := range fc.VirtualMachines {
		fmt.Fprintf(w, "libvirt_domain_info_memory_usage_bytes{domain=\"%s\",host=\"%s\"} %.0f\n", vm.Name, vm.HypervisorName, vm.MemoryUsageBytes)
		fmt.Fprintf(w, "libvirt_domain_info_maximum_memory_bytes{domain=\"%s\",host=\"%s\"} %.0f\n", vm.Name, vm.HypervisorName, vm.MemoryCapacityBytes)
		fmt.Fprintf(w, "libvirt_domain_info_virtual_cpus{domain=\"%s\",host=\"%s\"} %.0f\n", vm.Name, vm.HypervisorName, vm.CpuCapacityCores)
		fmt.Fprintf(w, "libvirt_domain_info_cpu_time_seconds_total{domain=\"%s\",host=\"%s\"} %.2f\n", vm.Name, vm.HypervisorName, vm.CpuTimeSecondsTotal)
	}
}

func NewFakeCluster() *FakeCluster {
	return &FakeCluster{Hypervisors: make(map[string]*model.Hypervisor), VirtualMachines: make(map[string]*SimVM)}
}

func (fc *FakeCluster) CreateHypervisor(name string, cpuCapacity float64, memoryCapacity float64) (*model.Hypervisor, error) {
	if name == "" {
		return nil, fmt.Errorf("Имя гипервизора не может быть пустым")
	}
	if cpuCapacity <= 0 || memoryCapacity <= 0 {
		return nil, fmt.Errorf("Количество ресурсов гипервизора не может быть отрицательным")
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if _, exist := fc.Hypervisors[name]; exist {
		return nil, fmt.Errorf("Гипервизор с таким именем уже есть в системе")
	}
	hv := &model.Hypervisor{Name: name, CpuCapacityCores: cpuCapacity, MemoryCapacityBytes: memoryCapacity}
	fc.Hypervisors[name] = hv
	return hv, nil
}

func (fc *FakeCluster) MigrateVM(vmName string, targetHost string) error {
	if vmName == "" || targetHost == "" {
		return fmt.Errorf("Имя VM и имя HV не могут быть пустыми")
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()

	vm, exists := fc.VirtualMachines[vmName]
	if !exists {
		return fmt.Errorf("VM %q не найдена", vmName)
	}
	dst, exists := fc.Hypervisors[targetHost]
	if !exists {
		return fmt.Errorf("Целевой HV %q не найден", targetHost)
	}
	if vm.HypervisorName == targetHost {
		return nil
	}
	if dst.MemoryUsageBytes+vm.MemoryUsageBytes > dst.MemoryCapacityBytes {
		return fmt.Errorf("Недостаточно RAM на %q для переноса", targetHost)
	}

	if src, ok := fc.Hypervisors[vm.HypervisorName]; ok {
		src.CpuUsageCores -= vm.CpuUsageCores
		src.MemoryUsageBytes -= vm.MemoryUsageBytes
		src.MachinesInside = slices.DeleteFunc(src.MachinesInside, func(name string) bool {
			return name == vmName
		})
	}

	vm.HypervisorName = targetHost
	dst.MachinesInside = append(dst.MachinesInside, vm.Name)
	dst.CpuUsageCores += vm.CpuUsageCores
	dst.MemoryUsageBytes += vm.MemoryUsageBytes

	return nil
}

func (fc *FakeCluster) HandleMigrate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VM     string `json:"vm"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Некорректный JSON", http.StatusBadRequest)
		return
	}
	if err := fc.MigrateVM(req.VM, req.Target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("[LAB] Выполнена миграция %s -> %s", req.VM, req.Target)
	w.WriteHeader(http.StatusOK)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cluster := NewFakeCluster()
	if err := seedCluster(cluster); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", cluster.GetData)
	mux.HandleFunc("POST /migrate", cluster.HandleMigrate)

	go cluster.RunSimulation(ctx)

	server := &http.Server{Addr: ":9177", Handler: mux, ReadTimeout: 5 * time.Second}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	log.Print("остановка сервера...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("ошибка остановки сервера: %v", err)
	}

}
