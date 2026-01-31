#!/bin/bash
KEY=$(xxd -p ts.key)
mkdir encrypted
mv *.ts encrypted/
mkdir decrypted
cd encrypted
IV='d6b8bcfb856e2c93b9a39ef921c76436'
#IV='0'
for file in *; do
	echo $file
	openssl aes-128-cbc -d -K $KEY -iv $IV -nosalt -in $file -out ../decrypted/$file
done

sleep 2
cd ..
cp ./decrypted/*.ts ./
echo 'run this to convert:'
echo 'ffmpeg -f concat -i ./list.txt -c copy -bsf:a aac_adtstoasc <FILENAME_OUT>'
