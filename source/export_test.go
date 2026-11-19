package main

import (
	"reflect"
	"testing"
)

func TestExportSelect(t *testing.T) {
	names := []string{"tb_Achievement_Script.res", "tb_item_script.res", "tb_UI_String.res", "tb_Option_String.res", "tb_Loading_String.res", "tb_Talk_String.res", "tb_SystemMail.res", "tb_Map.res"}
	jp := exportSelect("jp", names)
	want := []string{"tb_Achievement_Script.res", "tb_item_script.res", "tb_UI_String.res", "tb_SystemMail.res"}
	if len(jp) != 1 || jp[0].dir != "res" || !reflect.DeepEqual(jp[0].files, want) {
		t.Fatalf("jp: %+v", jp)
	}
	kr := exportSelect("kr", names)
	if len(kr[0].files) != 6 { // Option/Loading/Talk stay for kr, SystemMail does not
		t.Fatalf("kr: %+v", kr)
	}
	gl := exportSelect("gb", []string{"tb_Shop_ENG.res", "tb_Help_ENG.res", "tb_UI_Img_ENG.res", "tb_Make_CHN.res", "tb_Item_CHN.res", "tb_Item_TWN.res", "tb_Help_PopUp_TWN.res"})
	if len(gl) != 3 || gl[0].dir != "ENG-res" || !reflect.DeepEqual(gl[0].files, []string{"tb_Shop_ENG.res"}) ||
		!reflect.DeepEqual(gl[1].files, []string{"tb_Item_TWN.res"}) || !reflect.DeepEqual(gl[2].files, []string{"tb_Item_CHN.res"}) {
		t.Fatalf("gb: %+v", gl)
	}
}
