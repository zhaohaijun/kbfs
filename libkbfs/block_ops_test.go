package libkbfs

import (
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"golang.org/x/net/context"
)

// rmdMatcher implements the gomock.Matcher interface to compare
// RootMetadata objects. We can't just compare pointers as copies are
// made for mutations.
type rmdMatcher struct {
	rmd *RootMetadata
}

// Matches returns whether x is a *RootMetadata and it has the same ID
// and latest key generation as m.rmd.
func (m rmdMatcher) Matches(x interface{}) bool {
	rmd, ok := x.(*RootMetadata)
	if !ok {
		return false
	}
	return (rmd.ID == m.rmd.ID) && (rmd.LatestKeyGeneration() == m.rmd.LatestKeyGeneration())
}

// String implements the Matcher interface for rmdMatcher.
func (m rmdMatcher) String() string {
	return fmt.Sprintf("Matches RMD %v", m.rmd)
}

func expectGetTLFCryptKeyForEncryption(config *ConfigMock, rmd *RootMetadata) {
	config.mockKeyman.EXPECT().GetTLFCryptKeyForEncryption(gomock.Any(),
		rmdMatcher{rmd}).Return(TLFCryptKey{}, nil)
}

func expectGetTLFCryptKeyForMDDecryption(config *ConfigMock, rmd *RootMetadata) {
	config.mockKeyman.EXPECT().GetTLFCryptKeyForMDDecryption(gomock.Any(),
		rmdMatcher{rmd}).Return(TLFCryptKey{}, nil)
}

// TODO: Add test coverage for decryption of blocks with an old key
// generation.

func expectGetTLFCryptKeyForBlockDecryption(
	config *ConfigMock, rmd *RootMetadata, blockPtr BlockPointer) {
	config.mockKeyman.EXPECT().GetTLFCryptKeyForBlockDecryption(gomock.Any(),
		rmdMatcher{rmd}, blockPtr).Return(TLFCryptKey{}, nil)
}

type TestBlock struct {
	A int
}

func blockOpsInit(t *testing.T) (mockCtrl *gomock.Controller,
	config *ConfigMock, ctx context.Context) {
	ctr := NewSafeTestReporter(t)
	mockCtrl = gomock.NewController(ctr)
	config = NewConfigMock(mockCtrl, ctr)
	bops := &BlockOpsStandard{config}
	config.SetBlockOps(bops)
	ctx = context.Background()
	return
}

func blockOpsShutdown(mockCtrl *gomock.Controller, config *ConfigMock) {
	config.ctr.CheckForFailures()
	mockCtrl.Finish()
}

func expectBlockEncrypt(config *ConfigMock, rmd *RootMetadata, decData Block, plainSize int, encData []byte, err error) {
	expectGetTLFCryptKeyForEncryption(config, rmd)
	config.mockCrypto.EXPECT().MakeRandomBlockCryptKeyServerHalf().
		Return(BlockCryptKeyServerHalf{}, nil)
	config.mockCrypto.EXPECT().UnmaskBlockCryptKey(
		BlockCryptKeyServerHalf{}, TLFCryptKey{}).Return(BlockCryptKey{}, nil)
	encryptedBlock := EncryptedBlock{
		EncryptedData: encData,
	}
	config.mockCrypto.EXPECT().EncryptBlock(decData, BlockCryptKey{}).
		Return(plainSize, encryptedBlock, err)
	if err == nil {
		config.mockCodec.EXPECT().Encode(encryptedBlock).Return(encData, nil)
	}
}

func expectBlockDecrypt(config *ConfigMock, rmd *RootMetadata, blockPtr BlockPointer, encData []byte, block TestBlock, err error) {
	expectGetTLFCryptKeyForBlockDecryption(config, rmd, blockPtr)
	config.mockCrypto.EXPECT().UnmaskBlockCryptKey(gomock.Any(), gomock.Any()).
		Return(BlockCryptKey{}, nil)
	config.mockCodec.EXPECT().Decode(encData, gomock.Any()).Return(nil)
	config.mockCrypto.EXPECT().DecryptBlock(gomock.Any(), BlockCryptKey{}, gomock.Any()).
		Do(func(encryptedBlock EncryptedBlock, key BlockCryptKey, b Block) {
		if b != nil {
			tb := b.(*TestBlock)
			*tb = block
		}
	}).Return(err)
}

func makeRMD() *RootMetadata {
	tlfID := FakeTlfID(0, false)
	return &RootMetadata{ID: tlfID}
}

func TestBlockOpsGetSuccess(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// expect one call to fetch a block, and one to decrypt it
	id := fakeBlockID(1)
	encData := []byte{1, 2, 3, 4}
	blockPtr := BlockPointer{ID: id}
	config.mockBserv.EXPECT().Get(ctx, id, blockPtr).Return(
		encData, BlockCryptKeyServerHalf{}, nil)
	decData := TestBlock{42}

	rmd := makeRMD()

	expectBlockDecrypt(config, rmd, blockPtr, encData, decData, nil)

	var gotBlock TestBlock
	err := config.BlockOps().Get(ctx, rmd, blockPtr, &gotBlock)
	if err != nil {
		t.Fatalf("Got error on get: %v", err)
	}

	if gotBlock != decData {
		t.Errorf("Got back wrong block data on get: %v", gotBlock)
	}
}

func TestBlockOpsGetFailGet(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// fail the fetch call
	id := fakeBlockID(1)
	err := errors.New("Fake fail")
	blockPtr := BlockPointer{ID: id}
	config.mockBserv.EXPECT().Get(ctx, id, blockPtr).Return(
		nil, BlockCryptKeyServerHalf{}, err)

	rmd := makeRMD()

	if err2 := config.BlockOps().Get(ctx, rmd, blockPtr, nil); err2 != err {
		t.Errorf("Got bad error: %v", err2)
	}
}

func TestBlockOpsGetFailDecryptBlockData(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// expect one call to fetch a block, then fail to decrypt i
	id := fakeBlockID(1)
	encData := []byte{1, 2, 3, 4}
	blockPtr := BlockPointer{ID: id}
	config.mockBserv.EXPECT().Get(ctx, id, blockPtr).Return(
		encData, BlockCryptKeyServerHalf{}, nil)
	err := errors.New("Fake fail")

	rmd := makeRMD()

	expectBlockDecrypt(config, rmd, blockPtr, encData, TestBlock{}, err)

	if err2 := config.BlockOps().Get(ctx, rmd, blockPtr, nil); err2 != err {
		t.Errorf("Got bad error: %v", err2)
	}
}

func TestBlockOpsReadySuccess(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// expect one call to encrypt a block, one to hash it
	decData := TestBlock{42}
	encData := []byte{1, 2, 3, 4}
	id := fakeBlockID(1)

	rmd := makeRMD()

	expectedPlainSize := 4
	expectBlockEncrypt(config, rmd, decData, expectedPlainSize, encData, nil)
	config.mockCrypto.EXPECT().MakePermanentBlockID(encData).Return(id, nil)

	id2, plainSize, readyBlockData, err :=
		config.BlockOps().Ready(ctx, rmd, decData)
	if err != nil {
		t.Errorf("Got error on ready: %v", err)
	} else if id2 != id {
		t.Errorf("Got back wrong id on ready: %v", id)
	} else if plainSize != expectedPlainSize {
		t.Errorf("Expected plainSize %d, got %d", expectedPlainSize, plainSize)
	} else if string(readyBlockData.buf) != string(encData) {
		t.Errorf("Got back wrong data on get: %v", readyBlockData.buf)
	}
}

func TestBlockOpsReadyFailTooLowByteCount(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// expect just one call to encrypt a block
	decData := TestBlock{42}
	encData := []byte{1, 2, 3}

	rmd := makeRMD()

	expectBlockEncrypt(config, rmd, decData, 4, encData, nil)

	_, _, _, err := config.BlockOps().Ready(ctx, rmd, decData)
	if _, ok := err.(TooLowByteCountError); !ok {
		t.Errorf("Unexpectedly did not get TooLowByteCountError; "+
			"instead got %v", err)
	}
}

func TestBlockOpsReadyFailEncryptBlockData(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// expect one call to encrypt a block, one to hash it
	decData := TestBlock{42}
	err := errors.New("Fake fail")

	rmd := makeRMD()

	expectBlockEncrypt(config, rmd, decData, 0, nil, err)

	if _, _, _, err2 := config.BlockOps().
		Ready(ctx, rmd, decData); err2 != err {
		t.Errorf("Got bad error on ready: %v", err2)
	}
}

func TestBlockOpsReadyFailMakePermanentBlockID(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// expect one call to encrypt a block, one to hash it
	decData := TestBlock{42}
	encData := []byte{1, 2, 3, 4}
	err := errors.New("Fake fail")

	rmd := makeRMD()

	expectBlockEncrypt(config, rmd, decData, 4, encData, nil)

	config.mockCrypto.EXPECT().MakePermanentBlockID(encData).Return(fakeBlockID(0), err)

	if _, _, _, err2 := config.BlockOps().
		Ready(ctx, rmd, decData); err2 != err {
		t.Errorf("Got bad error on ready: %v", err2)
	}
}

func TestBlockOpsPutNewBlockSuccess(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// expect one call to put a block
	id := fakeBlockID(1)
	encData := []byte{1, 2, 3, 4}
	blockPtr := BlockPointer{ID: id}

	rmd := makeRMD()

	readyBlockData := ReadyBlockData{
		buf: encData,
	}

	config.mockBserv.EXPECT().Put(ctx, id, rmd.ID, blockPtr,
		readyBlockData.buf, readyBlockData.serverHalf).Return(nil)

	if err := config.BlockOps().
		Put(ctx, rmd, blockPtr, readyBlockData); err != nil {
		t.Errorf("Got error on put: %v", err)
	}
}

func TestBlockOpsPutIncRefSuccess(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// expect one call to put a block
	id := fakeBlockID(1)
	encData := []byte{1, 2, 3, 4}
	nonce := BlockRefNonce([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	blockPtr := BlockPointer{ID: id, RefNonce: nonce}

	rmd := makeRMD()

	readyBlockData := ReadyBlockData{
		buf: encData,
	}

	config.mockBserv.EXPECT().AddBlockReference(ctx, id, rmd.ID, blockPtr).
		Return(nil)

	if err := config.BlockOps().
		Put(ctx, rmd, blockPtr, readyBlockData); err != nil {
		t.Errorf("Got error on put: %v", err)
	}
}

func TestBlockOpsPutFail(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// fail the put call
	id := fakeBlockID(1)
	encData := []byte{1, 2, 3, 4}
	blockPtr := BlockPointer{ID: id}

	err := errors.New("Fake fail")

	rmd := makeRMD()

	readyBlockData := ReadyBlockData{
		buf: encData,
	}

	config.mockBserv.EXPECT().Put(ctx, id, rmd.ID, blockPtr,
		readyBlockData.buf, readyBlockData.serverHalf).Return(err)

	if err2 := config.BlockOps().
		Put(ctx, rmd, blockPtr, readyBlockData); err2 != err {
		t.Errorf("Got bad error on put: %v", err2)
	}
}

func TestBlockOpsDeleteSuccess(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// expect one call to delete a block
	rmd := makeRMD()

	id := fakeBlockID(1)
	blockPtr := BlockPointer{ID: id}
	config.mockBserv.EXPECT().RemoveBlockReference(ctx, id, rmd.ID, blockPtr).
		Return(nil)

	if err := config.BlockOps().Delete(ctx, rmd, id, blockPtr); err != nil {
		t.Errorf("Got error on put: %v", err)
	}
}

func TestBlockOpsDeleteFail(t *testing.T) {
	mockCtrl, config, ctx := blockOpsInit(t)
	defer blockOpsShutdown(mockCtrl, config)

	// fail the delete call
	rmd := makeRMD()

	id := fakeBlockID(1)
	err := errors.New("Fake fail")
	blockPtr := BlockPointer{ID: id}
	config.mockBserv.EXPECT().RemoveBlockReference(ctx, id, rmd.ID, blockPtr).
		Return(err)

	if err2 := config.BlockOps().Delete(ctx, rmd, id, blockPtr); err2 != err {
		t.Errorf("Got bad error on put: %v", err2)
	}
}
