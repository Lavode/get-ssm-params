package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

const appVersion = "1.0.0"

func main() {
	var defaultRegion string
	if os.Getenv("SSM_AWS_REGION") == "" {
		defaultRegion = "eu-central-1"
	} else {
		defaultRegion = os.Getenv("SSM_AWS_REGION")
	}
	var environment = flag.String("env", os.Getenv("SSM_ENV"), "[$SSM_ENV] environment name to use (PROD, STAG, ...)")
	var service = flag.String("service", os.Getenv("SSM_SERVICE"), "[$SSM_SERVICE] service name to use (YVES, ZED, ...)")
	var envparams = flag.String("params", os.Getenv("SSM_PARAMS"), "[$SSM_PARAMS] parameters to fetch (prefixes env+service)")
	var extraparams = flag.String("extraparams", os.Getenv("SSM_EXTRA_PARAMS"), "[$SSM_EXTRA_PARAMS] parameters to fetch (explicit)")
	var awsregion = flag.String("awsregion", defaultRegion, "[$SSM_AWS_REGION] AWS region")
	var printVersion = flag.Bool("version", false, "Print version of get-ssm-params")
	flag.Parse()

	if *printVersion {
		fmt.Println(appVersion)
		os.Exit(0)
	}
	os.Setenv("AWS_REGION", *awsregion)

	// retrieve desired parameters via API
	sess := session.Must(session.NewSession())
	svc := ssm.New(sess)
	params := &ssm.GetParametersInput{
		Names:          cliParams(*environment, *service, *envparams, *extraparams),
		WithDecryption: aws.Bool(true),
	}
	resp, err := svc.GetParameters(params)

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// exit(1) if user requested parameters that do not exist (fail early...)
	if len(resp.InvalidParameters) > 0 {
		fmt.Println("ERROR: Invalid/Unavailable parameter(s):")
		for _, p := range resp.InvalidParameters {
			fmt.Printf("  - %s\n", *p)
		}
		os.Exit(1)
	}

	if len(flag.Args()) == 0 {
		// print all the valid parameters we have, ready-to-source
		for _, p := range resp.Parameters {
			fmt.Printf(`%s="%s"`+"\n", stripEnvAndService(*p.Name, *environment, *service), *p.Value)
		}
	} else {
		// execute given entrypoint with parameters added to environment
		for _, p := range resp.Parameters {
			os.Setenv(stripEnvAndService(*p.Name, *environment, *service), *p.Value)
		}
		env := os.Environ()
		execErr := syscall.Exec(flag.Args()[0], flag.Args(), env)
		if execErr != nil {
			panic(execErr)
		}
	}
}

// cliParams builds and returns array of parameters to retrieve, based on command line args
func cliParams(env string, service string, params string, extraparams string) []*string {
	p := []*string{}

	// handle env/svc specific -params
	if params != "" {
		if env == "" || service == "" {
			fmt.Println("ERROR: -env and -service must be given for -params to work")
			os.Exit(1)
		}
		pp := strings.Split(params, ",")
		for _, arg := range pp {
			p = append(p, aws.String(fmt.Sprintf("%s_%s_%s", env, service, arg)))
		}
	}

	// handle -extraparams; again, comma-separated
	if extraparams != "" {
		pp := strings.Split(extraparams, ",")
		for _, arg := range pp {
			p = append(p, aws.String(arg))
		}
	}

	if len(p) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  get-ssm-params [-env ENV] [-service SVC] [-params PARAMS] [-extraparams XPARAMS] [...command...]")
		fmt.Println(" or")
		fmt.Println("  SSM_PARAMS='foo,bar' SSM_ENV=PROD SSM_SERVICE=MINE SSM_ENTRYPOINT='npm run prod' get-ssm-params [...command...]")
		os.Exit(1)
	}

	return p
}

// stripEnvAndService strips environment name and service name from <in>,
// eg. PROD_FOO_DB_USER -> DB_USER
func stripEnvAndService(in string, env string, service string) string {
	reg, _ := regexp.Compile("^" + env + "_" + service + "_")
	return reg.ReplaceAllString(in, "")
}
