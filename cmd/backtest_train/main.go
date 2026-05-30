package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/run-bigpig/jcp/internal/backtest"
	"github.com/run-bigpig/jcp/internal/models"
	"github.com/run-bigpig/jcp/internal/services"
)

const (
	initialCapital = 1000000.0 // 100万
	maxTrades      = 10        // 最大交易次数
	testDays       = 50        // 回测天数
	warmupDays     = 60        // 特征计算预热
	holdDays       = 3         // 持有天数
	trainDays      = 150       // 用于训练的历史天数
)

type stockKline struct {
	code   string
	name   string
	klines []models.KLineData
}

type trade struct {
	stockName string
	stockCode string
	buyDay    int
	buyPrice  float64
	sellDay   int
	sellPrice float64
	returns   float64
	profit    float64
	signal    string
}

type dailySignal struct {
	day       int
	code      string
	name      string
	predRet   float64
	signal    string
	confidence float64
}

func main() {
	fmt.Println("========================================")
	fmt.Println("  K线训练营自动回测")
	fmt.Println("========================================")
	fmt.Printf("初始资金: %.0f 元\n", initialCapital)
	fmt.Printf("最大交易: %d 次\n", maxTrades)
	fmt.Printf("回测周期: %d 个交易日\n", testDays)
	fmt.Println()

	// 1. 获取股票
	stocks := loadStocks()
	fmt.Printf("股票池: %d 支\n", len(stocks))

	// 2. 获取K线数据
	var allData []stockKline
	for _, s := range stocks {
		fmt.Printf("获取 %s %s ...", s.Symbol, s.Name)
		klines, err := services.NewMarketService().GetKLineData(s.Symbol, "1d", trainDays+testDays+warmupDays+holdDays+10)
		if err != nil || len(klines) < trainDays+warmupDays {
			fmt.Println(" 跳过")
			continue
		}
		fmt.Printf(" OK (%d天)\n", len(klines))
		allData = append(allData, stockKline{s.Symbol, s.Name, klines})
	}
	fmt.Printf("有效股票: %d 支\n\n", len(allData))

	// 3. 训练模型（用前 trainDays 天数据）
	fmt.Println("--- 训练预测模型 ---")
	model, scaler := trainModel(allData, trainDays)
	if model == nil {
		fmt.Println("模型训练失败")
		return
	}
	fmt.Println("模型训练完成\n")

	// 4. 模拟回测
	fmt.Println("--- 开始回测 ---")
	cash := initialCapital
	var holding *trade
	var closedTrades []trade
	var equityCurve []float64

	// 回测每天
	for day := 0; day < testDays; day++ {
		// 如果持仓，检查是否到期卖出
		if holding != nil && day >= holding.buyDay+holdDays {
			// 找到卖出价
			sellPrice := getPriceAtDay(allData, holding.stockCode, trainDays+day)
			if sellPrice > 0 {
				ret := (sellPrice - holding.buyPrice) / holding.buyPrice
				profit := cash * ret
				cash += profit
				closedTrades = append(closedTrades, trade{
					stockName:  holding.stockName,
					stockCode:  holding.stockCode,
					buyDay:     holding.buyDay,
					buyPrice:   holding.buyPrice,
					sellDay:    day,
					sellPrice:  sellPrice,
					returns:    ret,
					profit:     profit,
					signal:     holding.signal,
				})
				fmt.Printf("[Day %2d] 卖出 %s @ %.2f, 收益: %+.2f%%, 盈亏: %+.0f\n",
					day, holding.stockName, sellPrice, ret*100, profit)
			}
			holding = nil
		}

		// 如果没有持仓且还有交易次数，寻找买入机会
		if holding == nil && len(closedTrades)/2 < maxTrades/2 {
			var candidates []dailySignal
			for _, sd := range allData {
				klines := getKlinesUpToDay(sd.klines, trainDays+day)
				if len(klines) < warmupDays+holdDays+5 {
					continue
				}
				features := backtest.ComputeFeatures(klines, warmupDays)
				if len(features) == 0 {
					continue
				}
				lastFeat := features[len(features)-1:]
				normFeat := scaler.Transform(lastFeat)
				pred := model.Predict(normFeat)[0]
				conf := math.Tanh(math.Abs(pred) / 0.005)

				sig := getSignal(pred, conf)
				if sig == "买入" || sig == "强买入" {
					candidates = append(candidates, dailySignal{
						day:        day,
						code:       sd.code,
						name:       sd.name,
						predRet:    pred,
						signal:     sig,
						confidence: conf,
					})
				}
			}

			// 按预测收益排序，选最强的
			if len(candidates) > 0 {
				sort.Slice(candidates, func(i, j int) bool {
					return candidates[i].predRet > candidates[j].predRet
				})
				best := candidates[0]
				buyPrice := getPriceAtDay(allData, best.code, trainDays+day)
				if buyPrice > 0 {
					holding = &trade{
						stockName: best.name,
						stockCode: best.code,
						buyDay:    day,
						buyPrice:  buyPrice,
						signal:    best.signal,
					}
					fmt.Printf("[Day %2d] 买入 %s @ %.2f, 信号: %s, 预测收益: %+.2f%%, 置信度: %.0f%%\n",
						day, best.name, buyPrice, best.signal, best.predRet*100, best.confidence*100)
				}
			}
		}

		// 记录权益
		equity := cash
		if holding != nil {
			curPrice := getPriceAtDay(allData, holding.stockCode, trainDays+day)
			if curPrice > 0 {
				equity = cash * (1 + (curPrice-holding.buyPrice)/holding.buyPrice)
			}
		}
		equityCurve = append(equityCurve, equity)
	}

	// 5. 输出结果
	printResults(closedTrades, equityCurve, cash, initialCapital)
}

func trainModel(allData []stockKline, trainDays int) (*backtest.GBMRegressor, *backtest.StandardScaler) {
	var allFeatures [][]float64
	var allReturns []float64

	for _, sd := range allData {
		trainKlines := sd.klines[:trainDays+warmupDays]
		adjusted := backtest.FilterExRights(trainKlines)
		if len(adjusted) < warmupDays+holdDays+10 {
			continue
		}
		features := backtest.ComputeFeatures(adjusted, warmupDays)
		if len(features) == 0 {
			continue
		}
		for j := 0; j < len(features) && j+holdDays < len(adjusted)-warmupDays; j++ {
			idx := warmupDays + j
			if idx+holdDays >= len(adjusted) {
				break
			}
			ret := (adjusted[idx+holdDays].Close - adjusted[idx].Close) / adjusted[idx].Close
			if math.Abs(ret) > 0.15 {
				continue
			}
			allFeatures = append(allFeatures, features[j])
			allReturns = append(allReturns, ret)
		}
	}

	if len(allFeatures) < 100 {
		return nil, nil
	}

	scaler := &backtest.StandardScaler{}
	scaler.Fit(allFeatures)
	normFeatures := scaler.Transform(allFeatures)

	model := backtest.NewGBMRegressor(backtest.GBMConfig{
		MaxDepth:     4,
		NEstimators:  200,
		LearningRate: 0.05,
		Lambda:       0.5,
		Gamma:        0.0,
		ColSample:    0.8,
		SubSample:    0.8,
		MinLeafSize:  50,
	})
	model.Fit(normFeatures, allReturns)

	return model, scaler
}

func getKlinesUpToDay(klines []models.KLineData, upTo int) []models.KLineData {
	if upTo >= len(klines) {
		return klines
	}
	return klines[:upTo]
}

func getPriceAtDay(allData []stockKline, code string, dayIdx int) float64 {
	for _, sd := range allData {
		if sd.code == code && dayIdx < len(sd.klines) {
			return sd.klines[dayIdx].Close
		}
	}
	return 0
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

func printResults(trades []trade, equityCurve []float64, finalCash, initial float64) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("         回测结果报告")
	fmt.Println("========================================")
	fmt.Printf("初始资金:   %.0f 元\n", initial)
	fmt.Printf("最终资金:   %.0f 元\n", finalCash)
	totalReturn := (finalCash - initial) / initial * 100
	fmt.Printf("总收益率:   %+.2f%%\n", totalReturn)
	fmt.Printf("总盈亏:     %+.0f 元\n", finalCash-initial)
	fmt.Println()

	// 交易统计
	winCount := 0
	lossCount := 0
	totalProfit := 0.0
	totalLoss := 0.0
	maxWin := 0.0
	maxLoss := 0.0
	winStreak := 0
	maxWinStreak := 0

	for _, t := range trades {
		if t.returns > 0 {
			winCount++
			totalProfit += t.profit
			if t.returns > maxWin {
				maxWin = t.returns
			}
			winStreak++
			if winStreak > maxWinStreak {
				maxWinStreak = winStreak
			}
		} else {
			lossCount++
			totalLoss += math.Abs(t.profit)
			if t.returns < maxLoss {
				maxLoss = t.returns
			}
			winStreak = 0
		}
	}

	fmt.Printf("交易次数:   %d 次 (赢%d / 亏%d)\n", len(trades), winCount, lossCount)
	winRate := 0.0
	if len(trades) > 0 {
		winRate = float64(winCount) / float64(len(trades)) * 100
	}
	fmt.Printf("胜率:       %.1f%%\n", winRate)

	pf := 0.0
	if totalLoss > 0 {
		pf = totalProfit / totalLoss
	}
	fmt.Printf("盈亏比:     %.2f\n", pf)
	fmt.Printf("最大单笔赢: %+.2f%%\n", maxWin*100)
	fmt.Printf("最大单笔亏: %+.2f%%\n", maxLoss*100)
	fmt.Printf("最大连胜:   %d 次\n", maxWinStreak)
	fmt.Println()

	// 每笔交易明细
	fmt.Println("--- 交易明细 ---")
	fmt.Printf("%-4s %-8s %-12s %8s %8s %8s %12s %6s\n",
		"序号", "股票", "信号", "买入价", "卖出价", "收益率", "盈亏", "天数")
	fmt.Println("---------------------------------------------------------------------------")
	for i, t := range trades {
		fmt.Printf("%-4d %-8s %-12s %8.2f %8.2f %+.2f%% %+.0f %d天\n",
			i+1, t.stockName, t.signal, t.buyPrice, t.sellPrice,
			t.returns*100, t.profit, t.sellDay-t.buyDay)
	}
	fmt.Println()

	// 权益曲线统计
	if len(equityCurve) > 0 {
		peak := equityCurve[0]
		maxDD := 0.0
		for _, e := range equityCurve {
			if e > peak {
				peak = e
			}
			dd := (peak - e) / peak
			if dd > maxDD {
				maxDD = dd
			}
		}
		fmt.Printf("最大回撤:   %.2f%%\n", maxDD*100)
	}

	// 年化收益（假设250交易日/年）
	if len(equityCurve) > 0 {
		annualized := totalReturn / float64(len(equityCurve)) * 250
		fmt.Printf("年化收益:   %.2f%% (按%d天推算)\n", annualized, len(equityCurve))
	}
}

func loadStocks() []struct {
	Symbol string
	Name   string
} {
	// 使用与训练相同的大市值股票池
	codes := []struct {
		Symbol string
		Name   string
	}{
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
	return codes
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
