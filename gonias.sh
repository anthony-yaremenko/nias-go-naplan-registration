# This is the *nix NIAS batch file launcher. Add extra validators to the bottom of this list. 
# Change the directory as appropriate (go-nias)
# gnatsd MUST be the first program launched

#rem Run the NIAS services. Add to the BOTTOM of this list
# store each PID in pid list
../../nats-io/gnatsd/gnatsd & echo $! > nias.pid


./aggregator/aggregator & echo $! >> nias.pid
./aslvalidator/aslvalidator & echo $! >> nias.pid
./idvalidator/idvalidator & echo $! >> nias.pid
./schemavalidator/schemavalidator & echo $! >> nias.pid
./dobvalidator/dobvalidator & echo $! >> nias.pid

echo "Run the web client (launch browser here):"
echo "http://localhost:1324/validation"

