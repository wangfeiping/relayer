package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"

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

	return cmd
}

func initChainsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init [keyName] [mnemonic]",
		Aliases: []string{"auto"},
		Short:   "auto init all chains",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("running cmd bot...")
			keyName := args[0]
			mnem := args[1]
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
