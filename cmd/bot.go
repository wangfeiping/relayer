package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/iqlusioninc/relayer/relayer"
	"github.com/spf13/cobra"
)

func botCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bot",
		Aliases: []string{"auto"},
		Short:   "auto running"}

	cmd.AddCommand(initChainsCmd())
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

func initChainsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init [chainID] [keyName] [mnemonic]",
		Aliases: []string{"auto"},
		Short:   "auto init all chains",
		Args:    cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("bot init...")
			chianID := args[0]
			keyName := args[1]
			mnem := args[2]
			configChange := false
			for i, c := range config.Chains {
				lite, key, path, bal := chainCheck(i, c)
				if !key {
					if err := initKey(c, keyName, mnem); err != nil {
						return err
					}
					configChange = true
				}
				if !bal {
					if err := testnetRequest(c, keyName); err != nil {
						fmt.Println("request faucet error: " + err.Error())
					}
				}
				if !lite {
					if err := liteInit(c, keyName); err != nil {
						fmt.Println("lite init error: " + err.Error())
					}
				}
				if !path {
					srcChain, err := config.Chains.Get(chianID)
					if err == nil {
						pathName, err := pathGen(srcChain, c)
						if err != nil {
							fmt.Println("gen path error: " + err.Error())
							return err
						}
						configChange = true
						fmt.Println("gen path: " + pathName)
						txLink(pathName)
					} else {
						fmt.Println("get src chain error: " + err.Error())
					}

				}
				chainCheck(i, c)
			}
			if configChange {
				return overWriteConfig(cmd, config)
			}
			return nil
		},
	}

	return cmd
}

func genKey(keyName string, chain *relayer.Chain) error {
	// fmt.Printf("key: %s; %s\n", chain.ChainID, keyName)
	// return nil
	done := chain.UseSDKContext()
	defer done()

	if chain.KeyExists(keyName) {
		return errKeyExists(keyName)
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

func chainCheck(i int, c *relayer.Chain) (lite, key, path, bal bool) {
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
	fmt.Printf("%2d: %-20s -> key(%s) bal(%s) lite(%s) path(%s)\n",
		i, c.ChainID,
		OkString(key), OkString(bal), OkString(lite), OkString(path))
	return
}

func OkString(status bool) string {
	if status {
		return "✔"
	}
	return "✘"
}
