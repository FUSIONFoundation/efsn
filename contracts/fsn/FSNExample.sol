pragma solidity ^0.5.4;

import "https://github.com/cross-chain/efsn/contracts/fsn/FSNContract.sol";

contract FSNExample is FSNContract {
    address owner;
    modifier onlyOwner {
        require(msg.sender == owner, "only owner");
        _;
    }

    constructor() public {
        owner = msg.sender;
    }

    // If a contract want to receive Fusion Asset and TimeLock from an EOA,
    // the contract must impl the following 'receiveAsset' interface.
    function receiveAsset(bytes32 assetID, uint64 startTime, uint64 endTime, SendAssetFlag flag, uint256[] memory extraInfo) payable public returns (bool success) {
        (assetID, startTime, endTime, flag, extraInfo); // silence warning of Unused function parameter
        return true;
    }

    // impl by calling a precompiled contract '0x9999999999999999999999999999999999999999'
    // which support send out Fusion Asset and TimeLock from the calling contract.
    function sendAsset(bytes32 asset, address to, uint256 value, uint64 start, uint64 end, SendAssetFlag flag) onlyOwner public returns (bool success) {
        (success,) = _sendAsset(asset, to, value, start, end, flag);
        require(success, "call sendAsset failed");
        return true;
    }
}
