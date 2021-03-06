# This is the *nix NIAS batch file launcher. Add extra validators to the bottom of this list. 
# Change the directory as appropriate (go-nias)
# gnatsd MUST be the first program launched

if [ -f "nias.pid" ]
then
echo "There is a nias.pid file in place; run shutdown.sh"
exit
fi

#rem Run the NIAS services. Add to the BOTTOM of this list
# store each PID in pid list
./gnatsd & echo $! > nias.pid

# give the nats server time to come up
sleep 2

./aggregator & echo $! >> nias.pid
./aslvalidator & echo $! >> nias.pid
./idvalidator & echo $! >> nias.pid
./schemavalidator & echo $! >> nias.pid
./csvxmlconverter & echo $! >> nias.pid
./dobvalidator -tstyr 2016 & echo $! >> nias.pid
./webui & echo $! >> nias.pid

echo "Run the web client (launch browser here):"
echo "http://localhost:8080/nias"

