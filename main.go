package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/ssm"
)

// AppVersion is set at compile time
var AppVersion = "0.0.0-dev"

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
	var roleARN = flag.String("rolearn", os.Getenv("SSM_ROLEARN"), "[$SSM_ROLE_ARN] use given IAM role (ARN)")
	var printVersion = flag.Bool("version", false, "Print version of get-ssm-params")
	var s3Get = flag.Bool("s3-get", false, "fetch file from S3, args: [bucket] [key] [localFile]")
	flag.Parse()

	if *printVersion {
		fmt.Println(AppVersion)
		os.Exit(0)
	}

	// just exec() if SSM_PARAMS not set -- used to run/test containers outside of ECS
	if *envparams == "" && len(flag.Args()) > 0 {
		fmt.Println("Notice: No SSM_PARAMS provided to get-ssm-params. Passing through to exec().")
		env := os.Environ()
		execErr := syscall.Exec(flag.Args()[0], flag.Args(), env)
		errorExit("Failed to execute", execErr)
	}

	os.Setenv("AWS_REGION", *awsregion)
	sess := session.Must(session.NewSession())

	// retrieve single file from S3 if '-s3-get' was given
	if *s3Get {
		if len(flag.Args()) == 3 {
			s3GetFile(sess, *roleARN, flag.Args()[0], flag.Args()[1], flag.Args()[2])
			os.Exit(0)
		}
		fmt.Println("Bad usage, try: get-ssm-params --s3-get bucket key localName")
		os.Exit(1)
	}

	// by default, get SSM parameters
	ssmGet(sess, *roleARN, environment, service, envparams, extraparams)
}

func s3GetFile(sess *session.Session, arn string, bucket, key, localPath string) {
	sessConfig := &aws.Config{}
	if arn != "" {
		creds := stscreds.NewCredentials(sess, arn)
		sessConfig = &aws.Config{Credentials: creds}
	}
	svc := s3.New(sess, sessConfig)

	var timeout = time.Second * 30
	ctx := context.Background()
	var cancelFn func()
	ctx, cancelFn = context.WithTimeout(ctx, timeout)
	defer cancelFn()
	var input = &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	o, err := svc.GetObjectWithContext(ctx, input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == request.CanceledErrorCode {
			errorExit("Download canceled due to timeout", err)
		} else {
			errorExit("Failed to download object", err)
		}
	}

	// slurp S3 file into memory, assuming small files...
	body, err := ioutil.ReadAll(o.Body)
	errorExit("reading body", err)

	// write file from memory to disk
	err = ioutil.WriteFile(localPath, body, 0644)
	errorExit(fmt.Sprintf("writing %s", localPath), err)

	fmt.Printf("SUCCESS Fetching 's3://%s/%s' -> '%s' (%d bytes)\n", bucket, key, localPath, *o.ContentLength)
}

func ssmGet(sess *session.Session, arn string, environment, service, envparams, extraparams *string) {
	sessConfig := &aws.Config{}
	if arn != "" {
		creds := stscreds.NewCredentials(sess, arn)
		sessConfig = &aws.Config{Credentials: creds}
	}
	svc := ssm.New(sess, sessConfig)
	params := &ssm.GetParametersInput{
		Names:          cliParams(*environment, *service, *envparams, *extraparams),
		WithDecryption: aws.Bool(true),
	}
	resp, err := svc.GetParameters(params)
	errorExit("GetParameters", err)

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
		errorExit("Failed to execute", execErr)
	}
}

// cliParams builds and returns array of parameters to retrieve, based on command line args
func cliParams(env string, service string, params string, extraparams string) []*string {
	p := []*string{}

	// handle env/svc specific -params
	if params != "" {
		if env == "" || service == "" {
			errorExit("bad arguments", errors.New("-env and -service must be given for -params to work"))
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
		errorExit("bad usage", errors.New("try -h for help"))
	}
	return p
}

// strnilipEnvAndService strips environment name and service name from <in>,
// eg. PROD_FOO_DB_USER -> DB_USER
func stripEnvAndService(in string, env string, service string) string {
	reg, _ := regexp.Compile("^" + env + "_" + service + "_")
	return reg.ReplaceAllString(in, "")
}

// errorExit bails out on error
func errorExit(msg string, e error) {
	if e != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s, %v\n", msg, e)
		os.Exit(1)
	}
}
