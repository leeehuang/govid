#!/bin/bash
cd $1
KEY=$(xxd -p ts.key)
mkdir encrypted
mv *.ts encrypted/
mkdir decrypted
cd encrypted

IV=$2
#IV='0'
for file in *; do
	echo $file
	openssl aes-128-cbc -d -K $KEY -iv $IV -nosalt -in $file -out ../decrypted/$file
done

sleep 2
cd ..
cp ./decrypted/*.ts ./


