package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/run-bigpig/jcp/internal/services"
)

func main() {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, "Library", "Application Support", "jcp")
	os.MkdirAll(dataDir, 0755)

	ms := services.NewMarketService()
	ps := services.NewPredictionService(dataDir)

	fmt.Println("========================================")
	fmt.Println("  AI 预测模型训练 (GBM + LSTM)")
	fmt.Println("========================================")
	fmt.Println()

	// 先备份旧模型
	for _, f := range []string{"prediction_model.json", "prediction_model_lstm.json"} {
		p := filepath.Join(dataDir, f)
		if _, err := os.Stat(p); err == nil {
			bak := p + ".bak"
			data, _ := os.ReadFile(p)
			os.WriteFile(bak, data, 0644)
			fmt.Printf("已备份旧模型: %s\n", bak)
		}
	}
	fmt.Println()

	codes := []string{
		"sh600519", "sh601318", "sz000858", "sh600036", "sz000001",
		"sh600276", "sh601012", "sz002714", "sh600887", "sz000333",
		"sh601888", "sz002475", "sh600900", "sh601398", "sh601939",
		"sh600030", "sz000002", "sh600000", "sh601166", "sz002304",
		"sh600809", "sz000568", "sh601668", "sh600690", "sz002415",
		"sh601899", "sh600585", "sz000725", "sh601601", "sh600050",
		"sz002352", "sh600016", "sh601288", "sz000063", "sh600309",
		"sz002230", "sh601628", "sh600009", "sz000100", "sh600048",
		"sz002049", "sh601688", "sh600570", "sz000651", "sh600436",
		"sz002142", "sh601225", "sz000338", "sh600010", "sz002601",
	}

	fmt.Printf("训练股票数: %d\n", len(codes))
	fmt.Println("开始训练（GBM + LSTM）...")
	fmt.Println()

	if err := ps.TrainOnFetcher(ms, codes, 300); err != nil {
		fmt.Printf("训练失败: %v\n", err)
		os.Exit(1)
	}

	stocks, samples := ps.GetTrainInfo()
	fmt.Printf("\n训练完成: %d支股票, %d样本\n", stocks, samples)

	// 保存 GBM
	if err := ps.SaveToFile(); err != nil {
		fmt.Printf("保存GBM模型失败: %v\n", err)
		os.Exit(1)
	}
	gbmPath := filepath.Join(dataDir, "prediction_model.json")
	if info, err := os.Stat(gbmPath); err == nil {
		fmt.Printf("GBM 模型已保存: %s (%.0f KB)\n", gbmPath, float64(info.Size())/1024)
	}

	// 保存 LSTM
	if err := ps.SaveLSTMToFile(); err != nil {
		fmt.Printf("保存LSTM模型失败: %v\n", err)
		os.Exit(1)
	}
	lstmPath := filepath.Join(dataDir, "prediction_model_lstm.json")
	if info, err := os.Stat(lstmPath); err == nil {
		fmt.Printf("LSTM 模型已保存: %s (%.0f KB)\n", lstmPath, float64(info.Size())/1024)
	}

	fmt.Println()
	fmt.Println("运行时预测将融合两个模型：GBM 60%% + LSTM 40%%")
}
