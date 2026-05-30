package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/run-bigpig/jcp/internal/backtest"
	"github.com/run-bigpig/jcp/internal/services"
)

const (
	initialCapital = 1000000.0
	maxTrades      = 10
	testDays       = 50
	warmupDays     = 60
	holdDays       = 3
	numSessions    = 10 // 模拟10轮训练营
)

type trade struct {
	stockName string
	stockCode string
	sessionID int
	buyDay    int
	buyPrice  float64
	sellDay   int
	sellPrice float64
	returns   float64
	profit    float64
	signal    string
}

func main() {
	fmt.Println("========================================")
	fmt.Println("  K线训练营模拟回测（随机选股）")
	fmt.Println("========================================")
	fmt.Printf("初始资金: %.0f 元\n", initialCapital)
	fmt.Printf("每轮最大交易: %d 次\n", maxTrades)
	fmt.Printf("每轮交易天数: %d 天\n", testDays)
	fmt.Printf("模拟轮数: %d 轮\n", numSessions)
	fmt.Println()

	// 加载预训练模型
	home, _ := os.UserHomeDir()
	modelPath := filepath.Join(home, "Library", "Application Support", "jcp", "prediction_model.json")
	model, scaler := loadModel(modelPath)
	if model == nil {
		fmt.Println("模型加载失败，请先运行 go run ./cmd/trainmodel/ 训练模型")
		return
	}
	fmt.Println("模型加载成功")

	// 获取所有股票
	ms := services.NewMarketService()
	allStocks := loadAllStocks()
	fmt.Printf("股票池: %d 支\n\n", len(allStocks))

	// 汇总
	totalTrades := 0
	totalWins := 0
	totalProfit := 0.0
	totalLoss := 0.0
	var allTrades []trade
	var sessionReturns []float64

	for session := 1; session <= numSessions; session++ {
		fmt.Printf("=== 第 %d 轮 ===\n", session)

		// 随机选一支股票
		stock := allStocks[rand.Intn(len(allStocks))]
		fmt.Printf("选股: %s %s\n", stock.Name, stock.Symbol)

		// 获取K线数据
		klines, err := ms.GetKLineData(stock.Symbol, "1d", warmupDays+testDays+holdDays+10)
		if err != nil || len(klines) < warmupDays+testDays {
			fmt.Printf("数据不足，跳过\n\n")
			continue
		}

		// 模拟交易
		cash := initialCapital
		var holding *trade
		var sessionTrades []trade

		for day := 0; day < testDays; day++ {
			// 持仓到期卖出
			if holding != nil && day >= holding.buyDay+holdDays {
				sellIdx := warmupDays + day
				if sellIdx < len(klines) {
					sellPrice := klines[sellIdx].Close
					ret := (sellPrice - holding.buyPrice) / holding.buyPrice
					profit := cash * ret
					cash += profit
					t := trade{
						stockName: holding.stockName,
						stockCode: holding.stockCode,
						sessionID: session,
						buyDay:    holding.buyDay,
						buyPrice:  holding.buyPrice,
						sellDay:   day,
						sellPrice: sellPrice,
						returns:   ret,
						profit:    profit,
						signal:    holding.signal,
					}
					sessionTrades = append(sessionTrades, t)
					fmt.Printf("  [Day %2d] 卖出 @ %.2f, 收益: %+.2f%%, 盈亏: %+.0f\n",
						day, sellPrice, ret*100, profit)
					holding = nil
				}
			}

			// 没有持仓，用AI预测决定是否买入
			if holding == nil && len(sessionTrades) < maxTrades {
				klinesUpTo := klines[:warmupDays+day]
				if len(klinesUpTo) >= warmupDays+holdDays+5 {
					features := backtest.ComputeFeatures(klinesUpTo, warmupDays)
					if len(features) > 0 {
						lastFeat := features[len(features)-1:]
						normFeat := scaler.Transform(lastFeat)
						pred := model.Predict(normFeat)[0]
						conf := math.Tanh(math.Abs(pred) / 0.005)
						sig := getSignal(pred, conf)

						// 只在买入信号时交易
						if sig == "买入" || sig == "强买入" {
							buyIdx := warmupDays + day
							if buyIdx < len(klines) {
								buyPrice := klines[buyIdx].Close
								holding = &trade{
									stockName: stock.Name,
									stockCode: stock.Symbol,
									buyDay:    day,
									buyPrice:  buyPrice,
									signal:    sig,
								}
								fmt.Printf("  [Day %2d] 买入 @ %.2f, 信号: %s, 预测: %+.2f%%\n",
									day, buyPrice, sig, pred*100)
							}
						}
					}
				}
			}
		}

		// 如果还有持仓，强制平仓
		if holding != nil {
			sellIdx := warmupDays + testDays - 1
			if sellIdx < len(klines) {
				sellPrice := klines[sellIdx].Close
				ret := (sellPrice - holding.buyPrice) / holding.buyPrice
				profit := cash * ret
				cash += profit
				sessionTrades = append(sessionTrades, trade{
					stockName: holding.stockName,
					stockCode: holding.stockCode,
					sessionID: session,
					buyDay:    holding.buyDay,
					buyPrice:  holding.buyPrice,
					sellDay:   testDays,
					sellPrice: sellPrice,
					returns:   ret,
					profit:    profit,
					signal:    holding.signal,
				})
				fmt.Printf("  [Day %2d] 强制平仓 @ %.2f, 收益: %+.2f%%\n",
					testDays, sellPrice, ret*100)
			}
		}

		// 统计本轮
		sessionReturn := (cash - initialCapital) / initialCapital * 100
		sessionReturns = append(sessionReturns, sessionReturn)
		allTrades = append(allTrades, sessionTrades...)

		for _, t := range sessionTrades {
			totalTrades++
			if t.returns > 0 {
				totalWins++
				totalProfit += t.profit
			} else {
				totalLoss += math.Abs(t.profit)
			}
		}

		fmt.Printf("本轮收益: %+.2f%%, 交易: %d 次\n\n", sessionReturn, len(sessionTrades))
	}

	// 输出汇总报告
	printSummary(allTrades, sessionReturns, totalTrades, totalWins, totalProfit, totalLoss)
}

func getSignal(predReturn, confidence float64) string {
	pct := predReturn * 100
	switch {
	case pct > 0.5 && confidence > 0.5:
		return "强买入"
	case pct > 0.15:
		return "买入"
	case pct < -0.5 && confidence > 0.5:
		return "强卖出"
	case pct < -0.15:
		return "卖出"
	default:
		return "观望"
	}
}

func loadModel(path string) (*backtest.GBMRegressor, *backtest.StandardScaler) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	var pf struct {
		Model  *backtest.GBMRegressorSnapshot `json:"model"`
		Scaler *backtest.ScalerSnapshot       `json:"scaler"`
	}
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, nil
	}
	model := &backtest.GBMRegressor{}
	model.LoadSnapshot(pf.Model)
	scaler := &backtest.StandardScaler{}
	scaler.LoadSnapshot(pf.Scaler)
	return model, scaler
}

func loadAllStocks() []struct{ Symbol, Name string } {
	return []struct{ Symbol, Name string }{
		{"sh600519", "贵州茅台"}, {"sh601318", "中国平安"}, {"sz000858", "五粮液"},
		{"sh600036", "招商银行"}, {"sz000001", "平安银行"}, {"sh600276", "恒瑞医药"},
		{"sh601012", "隆基绿能"}, {"sz002714", "牧原股份"}, {"sh600887", "伊利股份"},
		{"sz000333", "美的集团"}, {"sh601888", "中国中免"}, {"sz002475", "立讯精密"},
		{"sh600900", "长江电力"}, {"sh601398", "工商银行"}, {"sh601939", "建设银行"},
		{"sh600030", "中信证券"}, {"sz000002", "万科A"}, {"sh600000", "浦发银行"},
		{"sh601166", "兴业银行"}, {"sz002304", "洋河股份"}, {"sh600809", "山西汾酒"},
		{"sz000568", "泸州老窖"}, {"sh601668", "中国建筑"}, {"sh600690", "海尔智家"},
		{"sz002415", "海康威视"}, {"sh601899", "紫金矿业"}, {"sh600585", "海螺水泥"},
		{"sz000725", "京东方A"}, {"sh601601", "中国太保"}, {"sh600050", "中国联通"},
		{"sz002352", "顺丰控股"}, {"sh600016", "民生银行"}, {"sh601288", "农业银行"},
		{"sz000063", "中兴通讯"}, {"sh600309", "万华化学"}, {"sz002230", "科大讯飞"},
		{"sh601628", "中国人寿"}, {"sh600009", "上海机场"}, {"sz000100", "TCL科技"},
		{"sh600048", "保利发展"}, {"sz002049", "紫光国微"}, {"sh601688", "华泰证券"},
		{"sh600570", "恒生电子"}, {"sz000651", "格力电器"}, {"sh600436", "片仔癀"},
		{"sz002142", "宁波银行"}, {"sh601225", "陕西煤业"}, {"sz000338", "潍柴动力"},
		{"sh600010", "包钢股份"}, {"sz002601", "龙蟒佰利"},
	}
}

func printSummary(trades []trade, sessionReturns []float64, totalTrades, totalWins int, totalProfit, totalLoss float64) {
	fmt.Println("========================================")
	fmt.Println("        汇总报告（随机选股）")
	fmt.Println("========================================")

	// 本轮统计
	winSessions := 0
	lossSessions := 0
	for _, r := range sessionReturns {
		if r > 0 {
			winSessions++
		} else {
			lossSessions++
		}
	}

	totalReturn := 0.0
	for _, r := range sessionReturns {
		totalReturn += r
	}
	avgReturn := totalReturn / float64(len(sessionReturns))

	fmt.Printf("模拟轮数:   %d 轮 (赢%d / 亏%d)\n", len(sessionReturns), winSessions, lossSessions)
	fmt.Printf("平均每轮收益: %+.2f%%\n", avgReturn)
	fmt.Printf("累计收益:   %+.2f%%\n", totalReturn)
	fmt.Println()

	// 交易统计
	winRate := 0.0
	if totalTrades > 0 {
		winRate = float64(totalWins) / float64(totalTrades) * 100
	}
	pf := 0.0
	if totalLoss > 0 {
		pf = totalProfit / totalLoss
	}

	fmt.Printf("总交易次数: %d 次 (赢%d / 亏%d)\n", totalTrades, totalWins, totalTrades-totalWins)
	fmt.Printf("胜率:       %.1f%%\n", winRate)
	fmt.Printf("盈亏比:     %.2f\n", pf)
	fmt.Println()

	// 交易明细
	fmt.Println("--- 交易明细 ---")
	fmt.Printf("%-4s %-4s %-10s %-8s %8s %8s %8s %12s\n",
		"序号", "轮", "股票", "信号", "买入价", "卖出价", "收益率", "盈亏")
	fmt.Println("------------------------------------------------------------------------")
	for i, t := range trades {
		fmt.Printf("%-4d %-4d %-10s %-8s %8.2f %8.2f %+.2f%% %+.0f\n",
			i+1, t.sessionID, t.stockName, t.signal, t.buyPrice, t.sellPrice,
			t.returns*100, t.profit)
	}

	// 每轮收益
	fmt.Println()
	fmt.Println("--- 每轮收益 ---")
	for i, r := range sessionReturns {
		icon := "✅"
		if r < 0 {
			icon = "❌"
		}
		fmt.Printf("  第%2d轮: %s %+.2f%%\n", i+1, icon, r)
	}

	// 汇总
	fmt.Println()
	totalPnL := totalProfit - totalLoss
	fmt.Printf("总盈亏:     %+.0f 元\n", totalPnL)
	fmt.Printf("每轮平均:   %+.0f 元\n", totalPnL/float64(len(sessionReturns)))

	// 最大回撤（按轮）
	peak := 0.0
	maxDD := 0.0
	cumReturn := 0.0
	for _, r := range sessionReturns {
		cumReturn += r
		if cumReturn > peak {
			peak = cumReturn
		}
		dd := peak - cumReturn
		if dd > maxDD {
			maxDD = dd
		}
	}
	fmt.Printf("最大回撤:   %.2f%%\n", maxDD)
}
