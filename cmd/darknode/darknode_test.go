package main_test

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/republicprotocol/go-do"
	. "github.com/republicprotocol/republic-go/cmd/darknode"
	"github.com/republicprotocol/republic-go/darknode"
	"github.com/republicprotocol/republic-go/darkocean"
	"github.com/republicprotocol/republic-go/ethereum/contracts"
	"github.com/republicprotocol/republic-go/ethereum/ganache"
	"github.com/republicprotocol/republic-go/identity"
	"github.com/republicprotocol/republic-go/order"
	"github.com/republicprotocol/republic-go/rpc"
	"github.com/republicprotocol/republic-go/stackint"
)

const (
	GanacheRPC                 = "http://localhost:8545"
	NumberOfDarkNodes          = 48
	NumberOfBootstrapDarkNodes = 5
	NumberOfOrders             = 10
)

var _ = Describe("DarkNode", func() {

	Context("when watching the ocean", func() {

		var darkNodeRegistry contracts.DarkNodeRegistry
		var darkNodes darknode.DarkNodes
		var ctxs []context.Context
		var cancels []context.CancelFunc
		var shutdown chan struct{}

		BeforeEach(func() {

			// Bind to the DarkNodeRegistry contract
			connection, err := ganache.Connect(GanacheRPC)
			Ω(err).ShouldNot(HaveOccurred())
			darkNodeRegistry, err = contracts.NewDarkNodeRegistry(context.Background(), connection, ganache.GenesisTransactor(), &bind.CallOpts{})
			Ω(err).ShouldNot(HaveOccurred())
			darkNodeRegistry.SetGasLimit(1000000)

			// Create DarkNodes and contexts/cancels for running them
			darkNodes, ctxs, cancels = NewLocalDarkNodes(NumberOfDarkNodes, NumberOfBootstrapDarkNodes)

			shutdown = make(chan struct{})

			var wg sync.WaitGroup
			wg.Add(len(darkNodes))
			for i := range darkNodes {
				go func(i int) {
					defer wg.Done()

					darkNodes[i].Run(ctxs[i])
				}(i)
			}

			go func() {
				defer close(shutdown)

				wg.Wait()
			}()

			// Wait for the DarkNodes to boot
			time.Sleep(time.Second)
		})

		AfterEach(func() {

			// Wait for the DarkNodes to shutdown
			<-shutdown
		})

		It("should converge on a global view of the ocean", func() {

			// Turn the epoch
			_, err := darkNodeRegistry.Epoch()
			Ω(err).ShouldNot(HaveOccurred())

			// Wait for DarkNodes to receive a notification and reconfigure
			// themselves
			time.Sleep(time.Second)

			// Verify that all DarkNodes have converged on the DarkOcean
			ocean, err := darkocean.NewOcean(darkNodeRegistry)
			Ω(err).ShouldNot(HaveOccurred())
			for i := range darkNodes {
				Ω(ocean.Equal(darkNodes[i].DarkOcean())).Should(BeTrue())
			}

			// Cancel all DarkNodes
			for i := range darkNodes {
				cancels[i]()
			}
		})

		It("should persist computations from recent epochs", func() {

		})

		It("should not persist computations from distant epochs", func() {

		})
	})

	FContext("when computing order matches", func() {

		var darkNodeRegistry contracts.DarkNodeRegistry
		var darkNodes darknode.DarkNodes
		var ctxs []context.Context
		var cancels []context.CancelFunc
		var shutdown chan struct{}

		BeforeEach(func() {

			// Bind to the DarkNodeRegistry contract
			connection, err := ganache.Connect(GanacheRPC)
			Ω(err).ShouldNot(HaveOccurred())
			darkNodeRegistry, err = contracts.NewDarkNodeRegistry(context.Background(), connection, ganache.GenesisTransactor(), &bind.CallOpts{})
			Ω(err).ShouldNot(HaveOccurred())
			darkNodeRegistry.SetGasLimit(1000000)

			// Create DarkNodes and contexts/cancels for running them
			darkNodes, ctxs, cancels = NewLocalDarkNodes(NumberOfDarkNodes, NumberOfBootstrapDarkNodes)

			shutdown = make(chan struct{})

			var wg sync.WaitGroup
			wg.Add(len(darkNodes))
			for i := range darkNodes {
				go func(i int) {
					defer wg.Done()

					darkNodes[i].Run(ctxs[i])
				}(i)
			}

			go func() {
				defer close(shutdown)

				wg.Wait()
			}()

			// Wait for the DarkNodes to boot
			time.Sleep(time.Second)
		})

		AfterEach(func() {

			// Wait for the DarkNodes to shutdown
			<-shutdown
		})

		It("should process the distribute order table in parallel with other pools", func() {
			By("start sending orders ")
			err := sendOrders(darkNodes, NumberOfOrders)
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("should update the order book after computing an order match", func() {

		})

	})

	Context("when confirming order matches", func() {

		It("should update the order book after confirming an order match", func() {

		})

		It("should update the order book after releasing an order match", func() {

		})
	})
})

func sendOrders(nodes darknode.DarkNodes, numberOfOrders int) error {
	log.Println(0)
	buyOrders, sellOrders := make([]*order.Order, numberOfOrders), make([]*order.Order, numberOfOrders)
	for i := 0; i < numberOfOrders; i++ {
		price := i * 1000000000000
		amount := i * 1000000000000
		sellOrder := order.NewOrder(order.TypeLimit, order.ParitySell, time.Now().Add(time.Hour),
			order.CurrencyCodeETH, order.CurrencyCodeBTC, stackint.FromUint(uint(price)), stackint.FromUint(uint(amount)),
			stackint.FromUint(uint(amount)), stackint.FromUint(1))
		sellOrders[i] = sellOrder

		buyOrder := order.NewOrder(order.TypeLimit, order.ParityBuy, time.Now().Add(time.Hour),
			order.CurrencyCodeETH, order.CurrencyCodeBTC, stackint.FromUint(uint(price)), stackint.FromUint(uint(amount)),
			stackint.FromUint(uint(amount)), stackint.FromUint(1))
		buyOrders[i] = buyOrder
	}
	log.Println(1)

	// Send order fragment to the nodes
	totalNodes := len(nodes)
	trader, _ := identity.NewMultiAddressFromString("/ip4/127.0.0.1/tcp/80/republic/8MGfbzAMS59Gb4cSjpm34soGNYsM2f")
	prime, _ := stackint.FromString("179769313486231590772930519078902473361797697894230657273430081157732675805500963132708477322407536021120113879871393357658789768814416622492847430639474124377767893424865485276302219601246094119453082952085005768838150682342462881473913110540827237163350510684586298239947245938479716304835356329624224137111")

	pool := rpc.NewClientPool(trader).WithTimeout(5 * time.Second).WithTimeoutBackoff(3 * time.Second)
	log.Println(2 )

	for i := range buyOrders {
		buyOrder, sellOrder := buyOrders[i], sellOrders[i]
		log.Printf("Sending matched order. [BUY] %s <---> [SELL] %s", buyOrder.ID, sellOrder.ID)
		buyShares, err := buyOrder.Split(int64(totalNodes), int64(totalNodes*2/3+1), &prime)
		if err != nil {
			return err
		}
		sellShares, err := sellOrder.Split(int64(totalNodes), int64(totalNodes*2/3+1), &prime)
		if err != nil {
			return err
		}

		do.CoForAll(buyShares, func(j int) {
			orderRequest := &rpc.OpenOrderRequest{
				From: &rpc.MultiAddress{
					Signature:    []byte{},
					MultiAddress: nodes[0].NetworkOption.MultiAddress.String(),
				},
				OrderFragment: rpc.MarshalOrderFragment(buyShares[j]),
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := pool.OpenOrder(ctx, nodes[j].NetworkOption.MultiAddress, orderRequest)
			if err != nil {
				log.Printf("Coudln't send order fragment to %s\n", nodes[j].NetworkOption.MultiAddress.ID())
				log.Fatal(err)
			}
		})

		do.CoForAll(sellShares, func(j int) {
			orderRequest := &rpc.OpenOrderRequest{
				From: &rpc.MultiAddress{
					Signature:    []byte{},
					MultiAddress: nodes[0].NetworkOption.MultiAddress.String(),
				},
				OrderFragment: rpc.MarshalOrderFragment(sellShares[j]),
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := pool.OpenOrder(ctx, nodes[j].NetworkOption.MultiAddress, orderRequest)
			if err != nil {
				log.Printf("Coudln't send order fragment to %s\n", nodes[j].NetworkOption.MultiAddress.ID())
				log.Fatal(err)
			}
		})
	}

	time.Sleep(time.Second)
	return nil
}