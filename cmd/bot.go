package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/iqlusioninc/relayer/exporter"
	"github.com/iqlusioninc/relayer/relayer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

var clientTimeRegexp = regexp.MustCompile(`"time":"(?P<time>.*?)"`)

func botCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bot",
		Aliases: []string{"b"},
		Short:   "auto running"}

	cmd.AddCommand(startPathCheckingCmd())
	cmd.AddCommand(genKeysCmd())
	return cmd
}

func genKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "genKeys [keyPrefix] [keyNumber]",
		Aliases: []string{"gk"},
		Short:   "gen 100keys for all chains",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("bot genKeys...")
			keyPrefix := args[0]
			keyNum, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				fmt.Println("bot genKeys error: " + err.Error())
				return err
			}
			for _, c := range config.Chains {
				for i := int64(0); i < keyNum; i++ {

					genKey(fmt.Sprintf("%s_%d", keyPrefix, i), c)
				}
			}
			return nil
		}}
	return cmd
}

func startPathCheckingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "start [path_name] [duration_seconds] [metrics_port]",
		Aliases: []string{"auto"},
		Short:   "auto check path",
		Args:    cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			fmt.Println("bot running...")

			path := args[0]
			sec, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				fmt.Println("parse sec error: " + err.Error())
				return
			}

			startExporter(args[2])
			fmt.Printf("exporter started\n")

			t := time.NewTicker(time.Duration(sec) * time.Second)

			pth, err := config.Paths.Get(path)
			if err != nil {
				fmt.Println("check path error: " + err.Error())
				return
			}
			chains, src, dst, err := config.ChainsFromPath(path)
			if err != nil {
				fmt.Println("check chains of path error: " + err.Error())
				return
			}
			srcChain := chains[src]
			dstChain := chains[dst]
			fmt.Printf("src: %s; dst: %s\n", src, dst)

			RPCs := []string{
				"35.233.155.199:26657",
				"http://34.83.218.4:26657",
				"http://34.83.90.172:26656",
				"http://47.74.39.90:27657",
				"http://47.103.79.28:36657"}
			GozHubID := "gameofzoneshub-1a"

			go func() {
				tq := time.NewTicker(time.Duration(60) * time.Second)
				for {
					select {
					case <-tq.C:
						{
							queryClient(srcChain, pth.Src.ClientID)
							queryClient(dstChain, pth.Dst.ClientID)
							exporter.SetStatusCode(
								1, time.Now().UTC().String(), "test")
							fmt.Println("query client done")
						}
					default:
						{
							time.Sleep(100 * time.Millisecond)
						}
					}
				}
			}()

			go func() {
				doCheck(srcChain, dstChain, pth, path,
					RPCs, GozHubID)
				for {
					select {
					case <-t.C:
						{
							doCheck(srcChain, dstChain, pth, path,
								RPCs, GozHubID)
						}
					default:
						{
							time.Sleep(100 * time.Millisecond)
						}
					}
				}
			}()

			done := func() { fmt.Println("bye") }
			trapSignal(done)
			return nil
		},
	}

	return cmd
}

func queryClient(c *relayer.Chain, clientID string) (err error) {
	if err = c.AddPath(clientID,
		dcon, dcha, dpor, dord); err != nil {
		fmt.Println("query client: %s %s; error: %v\n",
			c.ChainID, clientID, err)
		return err
	}
	res, err := c.QueryClientState()
	if err != nil {
		fmt.Println("query client: %s %s; error: %v\n",
			c.ChainID, clientID, err)
		return err
	}
	out, err := c.Amino.MarshalJSON(res)
	if err != nil {
		fmt.Println("query client: %s %s; error: %v\n",
			c.ChainID, clientID, err)
		return err
	}
	// fmt.Printf("query client: %s", string(out))
	dates := clientTimeRegexp.FindStringSubmatch(string(out))
	if len(dates) > 1 {
		exporter.SetStatusCode(1, dates[1], clientID)
	}
	return nil
}

func startExporter(listen string) {
	go func() {
		prometheus.MustRegister(exporter.Collector())

		http.Handle("/metrics", promhttp.Handler())
		err := http.ListenAndServe(listen, nil)
		if err != nil {
			fmt.Printf("start exporter error: %v\n", err)
			panic(err)
			return
		}
	}()
	return
}

func doCheck(src, dst *relayer.Chain,
	pth *relayer.Path, path string,
	rpcs []string, GozHubID string) {
	checking(src, rpcs, GozHubID)
	checking(dst, rpcs, GozHubID)

	updating(src, dst, path, pth.Src.ClientID, rpcs, GozHubID)
	updating(dst, src, path, pth.Dst.ClientID, rpcs, GozHubID)

	var timer time.Time
	timer = time.Now()
	fmt.Printf("All done. time(utc): %s\n", timer.UTC().String())
}

func updating(src, dst *relayer.Chain,
	path string, clientID string,
	rpcs []string, GozHubID string) {
	fmt.Printf("client updating: src: %s; dst: %s; %s %s\n",
		src.ChainID, dst.ChainID,
		path, clientID)
	i := 0
	err := updateClient(src, dst, clientID)
	for err != nil {
		time.Sleep(time.Duration(10) * time.Second)
		fmt.Println("re-try update client...")
		if src.ChainID == GozHubID {
			fmt.Printf("[ERR] client update: src: %s; dst: %s; %s %s; error: %v\n",
				src.ChainID, dst.ChainID,
				path, clientID, err)
			src.RPCAddr = getRpc(rpcs, i)
			reValidateConfig(src)
			err = updateClient(src, dst, clientID)
			i++
		} else {
			err = updateClient(src, dst, clientID)
		}
	}
	fmt.Printf("client updated: src: %s; dst: %s; %s %s\n",
		src.ChainID, dst.ChainID,
		path, clientID)
}

func checking(c *relayer.Chain, rpcs []string, GozHubID string) {
	i := 0
	err := checkingLite(c, GozHubID)
	for err != nil {
		time.Sleep(time.Duration(10) * time.Second)
		fmt.Println("re-try checking...")
		if c.ChainID == GozHubID {
			c.RPCAddr = getRpc(rpcs, i)
			reValidateConfig(c)
			err = checkingLite(c, GozHubID)
			i++
		} else {
			err = checkingLite(c, GozHubID)
		}
	}
	chainStatus(c)
}

func getRpc(rpcs []string, i int) string {
	l := len(rpcs)
	return rpcs[i%l]
}

// Called to initialize the relayer.Chain types on Config
// change RPC
func reValidateConfig(c *relayer.Chain) error {
	to, err := time.ParseDuration(config.Global.Timeout)
	if err != nil {
		fmt.Printf("[ERR] re-validate config %s; RPC: %s; error: %v\n",
			c.ChainID, c.RPCAddr, err)
		return err
	}

	if err := c.Init(homePath, appCodec, cdc, to, debug); err != nil {
		fmt.Printf("[ERR] re-validate config %s; RPC: %s; error: %v\n",
			c.ChainID, c.RPCAddr, err)
		return err
	}

	return nil
}

func checkingLite(c *relayer.Chain,
	GozHubID string) (err error) {
	fmt.Printf("lite checking %s; RPC: %s\n",
		c.ChainID, c.RPCAddr)
	if c.ChainID != GozHubID {
		if err = testnetRequest(c, c.Key); err != nil {
			fmt.Printf("[ERR] request faucet %s; RPC: %s; error: %v\n",
				c.ChainID, c.RPCAddr, err)
			return
		}
	}
	if err = liteInit(c, c.Key); err != nil {
		fmt.Printf("[ERR] lite init %s; RPC: %s; error: %v\n",
			c.ChainID, c.RPCAddr, err)
		return
	}
	// err = txLink(path)

	fmt.Printf("lite checked %s; RPC: %s\n", c.ChainID, c.RPCAddr)
	return
}

func updateClient(src, dst *relayer.Chain, clientID string) error {
	var err error
	if err = src.AddPath(clientID, dcon, dcha, dpor, dord); err != nil {
		return err
	}

	dstHeader, err := dst.UpdateLiteWithHeader()
	if err != nil {
		return err
	}

	// return sendAndPrint([]sdk.Msg{src.PathEnd.UpdateClient(dstHeader, src.MustGetAddress())}, src, nil)
	return send([]sdk.Msg{src.PathEnd.UpdateClient(dstHeader, src.MustGetAddress())}, src)
}

// SendAndPrint sends a transaction and prints according to the passed args
func send(txs []sdk.Msg, c *relayer.Chain) (err error) {
	text, indent := false, false
	// if c.debug {
	// 	if err = c.Print(txs, text, indent); err != nil {
	// 		return err
	// 	}
	// }
	// SendAndPrint sends the transaction with printing options from the CLI
	res, err := c.SendMsgs(txs)
	if err != nil {
		return err
	}
	if res.Height == 0 {
		return fmt.Errorf("height=%d", res.Height)
	}

	return c.Print(res, text, indent)

}

func genKey(keyName string, chain *relayer.Chain) error {
	// fmt.Printf("key: %s; %s\n", chain.ChainID, keyName)
	// return nil
	done := chain.UseSDKContext()
	defer done()

	if chain.KeyExists(keyName) {
		return nil //errKeyExists(keyName)
	}

	mnemonic, err := relayer.CreateMnemonic()
	if err != nil {
		return err
	}

	info, err := chain.Keybase.NewAccount(keyName, mnemonic, "", hd.CreateHDPath(118, 0, 0).String(), hd.Secp256k1)
	if err != nil {
		return err
	}

	ko := keyOutput{Mnemonic: mnemonic, Address: info.GetAddress().String()}

	return chain.Print(ko, false, false)
}

func txLink(path string) error {
	c, src, dst, err := config.ChainsFromPath(path)
	if err != nil {
		return err
	}

	// to, err := getTimeout(cmd)
	to, err := time.ParseDuration("30s")
	if err != nil {
		return err
	}

	if err = c[src].CreateClients(c[dst]); err != nil {
		return err
	}

	if err = c[src].CreateConnection(c[dst], to); err != nil {
		return err
	}

	return c[src].CreateChannel(c[dst], true, to)
}

func pathGen(srcChain, dstChain *relayer.Chain) (string, error) {
	// src, srcPort, dst, dstPort := args[0], args[1], args[2], args[3]
	src, srcPort, dst, dstPort := srcChain.ChainID, "transfer",
		dstChain.ChainID, "transfer"
	path := &relayer.Path{
		Src: &relayer.PathEnd{
			ChainID: src,
			PortID:  srcPort,
		},
		Dst: &relayer.PathEnd{
			ChainID: dst,
			PortID:  dstPort,
		},
		Strategy: &relayer.StrategyCfg{
			Type: "naive",
		},
	}
	// c, err := config.Chains.Gets(src, dst)
	// if err != nil {
	// 	return "", fmt.Errorf("chains need to be configured before paths to them can be added: %w", err)
	// }

	// unordered, err := cmd.Flags().GetBool(flagOrder)
	// if err != nil {
	// 	return err
	// }

	// if unordered {
	// 	path.Src.Order = "UNORDERED"
	// 	path.Dst.Order = "UNORDERED"
	// } else {
	// 	path.Src.Order = "ORDERED"
	// 	path.Dst.Order = "ORDERED"
	// }
	path.Src.Order = "ORDERED"
	path.Dst.Order = "ORDERED"

	// force, err := cmd.Flags().GetBool(flagForce)
	// if err != nil {
	// 	return err
	// }
	force := true

	pathName := src + "2" + dst

	if force {
		path.Dst.ClientID = relayer.RandLowerCaseLetterString(10)
		path.Src.ClientID = relayer.RandLowerCaseLetterString(10)
		path.Src.ConnectionID = relayer.RandLowerCaseLetterString(10)
		path.Dst.ConnectionID = relayer.RandLowerCaseLetterString(10)
		path.Src.ChannelID = relayer.RandLowerCaseLetterString(10)
		path.Dst.ChannelID = relayer.RandLowerCaseLetterString(10)
		if err := config.Paths.AddForce(pathName, path); err != nil {
			return "", err
		}
	}
	return pathName, nil

	// srcClients, err := c[src].QueryClients(1, 1000)
	// if err != nil {
	// 	return err
	// }

	// for _, c := range srcClients {
	// 	// TODO: support other client types through a switch here as they become available
	// 	clnt, ok := c.(tmclient.ClientState)
	// 	if ok && clnt.LastHeader.Commit != nil && clnt.LastHeader.Header != nil {
	// 		if clnt.GetChainID() == dst && !clnt.IsFrozen() {
	// 			path.Src.ClientID = c.GetID()
	// 		}
	// 	}
	// }

	// dstClients, err := c[dst].QueryClients(1, 1000)
	// if err != nil {
	// 	return err
	// }

	// for _, c := range dstClients {
	// 	// TODO: support other client types through a switch here as they become available
	// 	clnt, ok := c.(tmclient.ClientState)
	// 	if ok && clnt.LastHeader.Commit != nil && clnt.LastHeader.Header != nil {
	// 		if c.GetChainID() == src && !c.IsFrozen() {
	// 			path.Dst.ClientID = c.GetID()
	// 		}
	// 	}
	// }

	// switch {
	// case path.Src.ClientID == "" && path.Dst.ClientID == "":
	// 	path.Src.ClientID = relayer.RandLowerCaseLetterString(10)
	// 	path.Dst.ClientID = relayer.RandLowerCaseLetterString(10)
	// 	path.Src.ConnectionID = relayer.RandLowerCaseLetterString(10)
	// 	path.Dst.ConnectionID = relayer.RandLowerCaseLetterString(10)
	// 	path.Src.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	path.Dst.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	if err = config.Paths.Add(args[4], path); err != nil {
	// 		return err
	// 	}
	// 	return overWriteConfig(cmd, config)
	// case path.Src.ClientID == "" && path.Dst.ClientID != "":
	// 	path.Src.ClientID = relayer.RandLowerCaseLetterString(10)
	// 	path.Src.ConnectionID = relayer.RandLowerCaseLetterString(10)
	// 	path.Dst.ConnectionID = relayer.RandLowerCaseLetterString(10)
	// 	path.Src.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	path.Dst.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	if err = config.Paths.Add(args[4], path); err != nil {
	// 		return err
	// 	}
	// 	return overWriteConfig(cmd, config)
	// case path.Dst.ClientID == "" && path.Src.ClientID != "":
	// 	path.Dst.ClientID = relayer.RandLowerCaseLetterString(10)
	// 	path.Src.ConnectionID = relayer.RandLowerCaseLetterString(10)
	// 	path.Dst.ConnectionID = relayer.RandLowerCaseLetterString(10)
	// 	path.Src.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	path.Dst.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	if err = config.Paths.Add(args[4], path); err != nil {
	// 		return err
	// 	}
	// 	return overWriteConfig(cmd, config)
	// }

	// srcConns, err := c[src].QueryConnections(1, 1000)
	// if err != nil {
	// 	return err
	// }

	// var srcCon connTypes.IdentifiedConnectionEnd
	// for _, c := range srcConns {
	// 	if c.Connection.ClientID == path.Src.ClientID {
	// 		srcCon = c
	// 		path.Src.ConnectionID = c.Identifier
	// 	}
	// }

	// dstConns, err := c[dst].QueryConnections(1, 1000)
	// if err != nil {
	// 	return err
	// }

	// var dstCon connTypes.IdentifiedConnectionEnd
	// for _, c := range dstConns {
	// 	if c.Connection.ClientID == path.Dst.ClientID {
	// 		dstCon = c
	// 		path.Dst.ConnectionID = c.Identifier
	// 	}
	// }

	// switch {
	// case path.Src.ConnectionID != "" && path.Dst.ConnectionID != "":
	// 	// If we have identified a connection, make sure that each end is the
	// 	// other's counterparty and that the connection is open. In the failure case
	// 	// we should generate a new connection identifier
	// 	dstCpForSrc := srcCon.Connection.Counterparty.ConnectionID == dstCon.Identifier
	// 	srcCpForDst := dstCon.Connection.Counterparty.ConnectionID == srcCon.Identifier
	// 	srcOpen := srcCon.Connection.GetState().String() == "OPEN"
	// 	dstOpen := dstCon.Connection.GetState().String() == "OPEN"
	// 	if !(dstCpForSrc && srcCpForDst && srcOpen && dstOpen) {
	// 		path.Src.ConnectionID = relayer.RandLowerCaseLetterString(10)
	// 		path.Dst.ConnectionID = relayer.RandLowerCaseLetterString(10)
	// 		path.Src.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 		path.Dst.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 		if err = config.Paths.Add(args[4], path); err != nil {
	// 			return err
	// 		}
	// 		return overWriteConfig(cmd, config)
	// 	}
	// default:
	// 	path.Src.ConnectionID = relayer.RandLowerCaseLetterString(10)
	// 	path.Dst.ConnectionID = relayer.RandLowerCaseLetterString(10)
	// 	path.Src.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	path.Dst.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	if err = config.Paths.Add(args[4], path); err != nil {
	// 		return err
	// 	}
	// 	return overWriteConfig(cmd, config)
	// }

	// srcChans, err := c[src].QueryChannels(1, 1000)
	// if err != nil {
	// 	return err
	// }

	// var srcChan chanTypes.IdentifiedChannel
	// for _, c := range srcChans {
	// 	if c.Channel.ConnectionHops[0] == path.Src.ConnectionID {
	// 		srcChan = c
	// 		path.Src.ChannelID = c.ChannelIdentifier
	// 	}
	// }

	// dstChans, err := c[dst].QueryChannels(1, 1000)
	// if err != nil {
	// 	return err
	// }

	// var dstChan chanTypes.IdentifiedChannel
	// for _, c := range dstChans {
	// 	if c.Channel.ConnectionHops[0] == path.Dst.ConnectionID {
	// 		dstChan = c
	// 		path.Dst.ChannelID = c.ChannelIdentifier
	// 	}
	// }

	// switch {
	// case path.Src.ChannelID != "" && path.Dst.ChannelID != "":
	// 	dstCpForSrc := srcChan.Channel.Counterparty.ChannelID == dstChan.ChannelIdentifier
	// 	srcCpForDst := dstChan.Channel.Counterparty.ChannelID == srcChan.ChannelIdentifier
	// 	srcOpen := srcChan.Channel.GetState().String() == "OPEN"
	// 	dstOpen := dstChan.Channel.GetState().String() == "OPEN"
	// 	srcPort := srcChan.PortIdentifier == path.Src.PortID
	// 	dstPort := dstChan.PortIdentifier == path.Dst.PortID
	// 	srcOrder := srcChan.Channel.Ordering.String() == path.Src.Order
	// 	dstOrder := dstChan.Channel.Ordering.String() == path.Dst.Order
	// 	if !(dstCpForSrc && srcCpForDst && srcOpen && dstOpen && srcPort && dstPort && srcOrder && dstOrder) {
	// 		path.Src.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 		path.Dst.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	}
	// 	if err = config.Paths.Add(args[4], path); err != nil {
	// 		return err
	// 	}
	// 	return overWriteConfig(cmd, config)
	// default:
	// 	path.Src.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	path.Dst.ChannelID = relayer.RandLowerCaseLetterString(10)
	// 	if err = config.Paths.Add(args[4], path); err != nil {
	// 		return err
	// 	}
	// 	return overWriteConfig(cmd, config)
	// }
}

func liteInit(chain *relayer.Chain, keyName string) error {
	db, df, err := chain.NewLiteDB()
	if err != nil {
		return err
	}
	defer df()

	// url, err := cmd.Flags().GetString(flagURL)
	// if err != nil {
	// 	return err
	// }
	// force, err := cmd.Flags().GetBool(flagForce)
	// if err != nil {
	// 	return err
	// }
	force := true

	// height, err := cmd.Flags().GetInt64(flags.FlagHeight)
	// if err != nil {
	// 	return err
	// }
	// hash, err := cmd.Flags().GetBytesHex(flagHash)
	// if err != nil {
	// 	return err
	// }

	switch {
	case force: // force initialization from trusted node
		_, err = chain.TrustNodeInitClient(db)
		if err != nil {
			return err
		}
	// case height > 0 && len(hash) > 0: // height and hash are given
	// 	_, err = chain.InitLiteClient(db, chain.TrustOptions(height, hash))
	// 	if err != nil {
	// 		return wrapInitFailed(err)
	// 	}
	// case len(url) > 0: // URL is given, query trust options
	// 	_, err := neturl.Parse(url)
	// 	if err != nil {
	// 		return wrapIncorrectURL(err)
	// 	}

	// 	to, err := queryTrustOptions(url)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	_, err = chain.InitLiteClient(db, to)
	// 	if err != nil {
	// 		return wrapInitFailed(err)
	// 	}
	default: // return error
		return errInitWrongFlags
	}
	return nil
}

func testnetRequest(chain *relayer.Chain, keyName string) error {
	done := chain.UseSDKContext()
	defer done()

	u, err := url.Parse(chain.RPCAddr)
	if err != nil {
		return err
	}

	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		return err
	}

	urlString := fmt.Sprintf("%s://%s:%d", u.Scheme, host, 8000)

	info, err := chain.Keybase.Key(keyName)
	if err != nil {
		return err
	}

	body, err := json.Marshal(relayer.FaucetRequest{Address: info.GetAddress().String(), ChainID: chain.ChainID})
	if err != nil {
		return err
	}

	resp, err := http.Post(urlString, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	fmt.Println(string(respBody))
	return nil
}

func initKey(chain *relayer.Chain, keyName, mnem string) (err error) {
	// !!! delete key first
	if chain.KeyExists(keyName) {
		err = chain.Keybase.Delete(keyName)
		if err != nil {
			panic(err)
		}
	}

	// restore(import) key
	done := chain.UseSDKContext()
	defer done()

	if chain.KeyExists(keyName) {
		return errKeyExists(keyName)
	}

	info, err := chain.Keybase.NewAccount(keyName, mnem,
		"", hd.CreateHDPath(118, 0, 0).String(), hd.Secp256k1)
	if err != nil {
		return err
	}

	fmt.Println(info.GetAddress().String())

	// set default key
	c, err := chain.Update("key", keyName)
	if err != nil {
		return
	}

	if err = config.DeleteChain(c.ChainID).AddChain(c); err != nil {
		return err
	}
	return
}

func chainStatus(c *relayer.Chain) (lite, key, path, bal bool) {
	_, err := c.GetAddress()
	if err == nil {
		key = true
	}

	coins, err := c.QueryBalance(c.Key)
	if err == nil && !coins.Empty() {
		bal = true
	}

	_, err = c.GetLatestLiteHeader()
	if err == nil {
		lite = true
	}

	for _, pth := range config.Paths {
		if pth.Src.ChainID == c.ChainID || pth.Dst.ChainID == c.ChainID {
			path = true
		}
	}
	fmt.Printf("%-20s -> key(%s) bal(%s) lite(%s) path(%s)\n",
		c.ChainID,
		OkString(key), OkString(bal), OkString(lite), OkString(path))
	return
}

func OkString(status bool) string {
	if status {
		return "✔"
	}
	return "✘"
}
