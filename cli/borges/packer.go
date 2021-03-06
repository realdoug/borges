package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/src-d/borges"
	"github.com/src-d/borges/storage"
	core "gopkg.in/src-d/core-retrieval.v0"
	"gopkg.in/src-d/core-retrieval.v0/repository"
	"gopkg.in/src-d/framework.v0/queue"
	"gopkg.in/src-d/go-billy.v4/osfs"
)

const (
	packerCmdName      = "pack"
	packerCmdShortDesc = "quickly pack remote or local repositories into siva files"
	packerCmdLongDesc  = ""
)

type packerCmd struct {
	loggerCmd
	File      string `long:"file" short:"f" required:"true" description:"file with the repositories to pack (one per line)"`
	OutputDir string `long:"to" default:"repositories" description:"path to store the packed siva files"`
	Timeout   string `long:"timeout" default:"30m" description:"time to wait to consider a job failed"`
	Workers   int    `long:"workers" default:"0" description:"number of workers to use, defaults to number of available processors"`
}

func (c *packerCmd) Execute(args []string) error {
	c.init()

	log.Info("initializing pack process", "file", c.File, "output", c.OutputDir)

	broker := queue.NewMemoryBroker()
	q, err := broker.Queue("jobs")
	if err != nil {
		return fmt.Errorf("unable to start an in-memory queue: %s", err)
	}

	store := storage.Local()
	timeout, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return fmt.Errorf("invalid format in the given `--timeout` flag: %s", err)
	}

	transactioner, err := c.newRootedTransactioner()
	if err != nil {
		return fmt.Errorf("unable to initialize rooted transactioner: %s", err)
	}

	wp := borges.NewArchiverWorkerPool(
		log,
		store,
		transactioner,
		borges.NewTemporaryCloner(core.TemporaryFilesystem()),
		core.Locking(),
		timeout,
	)

	if c.Workers <= 0 {
		c.Workers = runtime.NumCPU()
	}
	wp.SetWorkerCount(c.Workers)

	f, err := os.Open(c.File)
	if err != nil {
		return fmt.Errorf("unable to open file %q with repositories: %s", c.File, err)
	}

	executor := borges.NewExecutor(
		log,
		q,
		wp,
		store,
		borges.NewLineJobIter(f, store),
	)

	return executor.Execute()
}

func (c *packerCmd) newRootedTransactioner() (repository.RootedTransactioner, error) {
	tmpFs, err := core.TemporaryFilesystem().Chroot("borges-packer")
	if err != nil {
		return nil, err
	}

	copier := repository.NewLocalCopier(osfs.New(c.OutputDir), 0)

	return repository.NewSivaRootedTransactioner(
		copier,
		tmpFs,
	), nil
}
