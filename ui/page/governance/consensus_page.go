package governance

import (
	"context"
	"time"

	"gioui.org/font/gofont"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"gitlab.com/raedah/cryptopower/app"
	"gitlab.com/raedah/cryptopower/libwallet"
	"gitlab.com/raedah/cryptopower/ui/cryptomaterial"
	"gitlab.com/raedah/cryptopower/ui/load"
	"gitlab.com/raedah/cryptopower/ui/modal"
	"gitlab.com/raedah/cryptopower/ui/page/components"
	"gitlab.com/raedah/cryptopower/ui/values"
)

const ConsensusPageID = "Consensus"

type ConsensusPage struct {
	*load.Load
	// GenericPageModal defines methods such as ID() and OnAttachedToNavigator()
	// that helps this Page satisfy the app.Page interface. It also defines
	// helper methods for accessing the PageNavigator that displayed this page
	// and the root WindowNavigator.
	*app.GenericPageModal

	ctx       context.Context // page context
	ctxCancel context.CancelFunc

	multiWallet    *libwallet.MultiWallet
	wallets        []*libwallet.Wallet
	LiveTickets    []*libwallet.Transaction
	consensusItems []*components.ConsensusItem

	listContainer       *widget.List
	syncButton          *widget.Clickable
	viewVotingDashboard *cryptomaterial.Clickable
	copyRedirectURL     *cryptomaterial.Clickable
	redirectIcon        *cryptomaterial.Image

	walletDropDown *cryptomaterial.DropDown
	orderDropDown  *cryptomaterial.DropDown
	consensusList  *cryptomaterial.ClickableList

	searchEditor cryptomaterial.Editor
	infoButton   cryptomaterial.IconButton

	syncCompleted bool
	isSyncing     bool
}

func NewConsensusPage(l *load.Load) *ConsensusPage {
	pg := &ConsensusPage{
		Load:             l,
		GenericPageModal: app.NewGenericPageModal(ConsensusPageID),
		multiWallet:      l.WL.MultiWallet,
		wallets:          l.WL.SortedWalletList(),
		consensusList:    l.Theme.NewClickableList(layout.Vertical),
		listContainer: &widget.List{
			List: layout.List{Axis: layout.Vertical},
		},
		syncButton: new(widget.Clickable),

		redirectIcon:        l.Theme.Icons.RedirectIcon,
		viewVotingDashboard: l.Theme.NewClickable(true),
		copyRedirectURL:     l.Theme.NewClickable(false),
	}

	pg.searchEditor = l.Theme.IconEditor(new(widget.Editor), values.String(values.StrSearch), l.Theme.Icons.SearchIcon, true)
	pg.searchEditor.Editor.SingleLine, pg.searchEditor.Editor.Submit, pg.searchEditor.Bordered = true, true, false

	_, pg.infoButton = components.SubpageHeaderButtons(l)
	pg.infoButton.Size = values.MarginPadding20

	pg.walletDropDown = components.CreateOrUpdateWalletDropDown(pg.Load, &pg.walletDropDown, pg.wallets, values.TxDropdownGroup, 0)
	pg.orderDropDown = components.CreateOrderDropDown(l, values.ConsensusDropdownGroup, 0)

	return pg
}

func (pg *ConsensusPage) OnNavigatedTo() {
	pg.ctx, pg.ctxCancel = context.WithCancel(context.TODO())
	pg.FetchAgendas()
}

func (pg *ConsensusPage) OnNavigatedFrom() {
	if pg.ctxCancel != nil {
		pg.ctxCancel()
	}
}

func (pg *ConsensusPage) HandleUserInteractions() {
	for pg.walletDropDown.Changed() {
		pg.FetchAgendas()
	}

	for pg.orderDropDown.Changed() {
		pg.FetchAgendas()
	}

	for i := range pg.consensusItems {
		if pg.consensusItems[i].VoteButton.Clicked() {
			voteModal := newAgendaVoteModal(pg.Load, &pg.consensusItems[i].Agenda, func() {
				go pg.FetchAgendas() // re-fetch agendas when modal is dismissed
			})
			pg.ParentWindow().ShowModal(voteModal)
		}
	}

	for pg.syncButton.Clicked() {
		go pg.FetchAgendas()
	}

	if pg.infoButton.Button.Clicked() {
		infoModal := modal.NewCustomModal(pg.Load).
			Title(values.String(values.StrConsensusChange)).
			Body(values.String(values.StrOnChainVote)).
			SetCancelable(true).
			PositiveButton(values.String(values.StrGotIt), modal.DefaultClickFunc())
		pg.ParentWindow().ShowModal(infoModal)
	}

	for pg.viewVotingDashboard.Clicked() {
		host := "https://voting.decred.org"
		if pg.WL.MultiWallet.NetType() == libwallet.Testnet3 {
			host = "https://voting.decred.org/testnet"
		}

		info := modal.NewCustomModal(pg.Load).
			Title(values.String(values.StrConsensusDashboard)).
			Body(values.String(values.StrCopyLink)).
			SetCancelable(true).
			UseCustomWidget(func(gtx C) D {
				return layout.Stack{}.Layout(gtx,
					layout.Stacked(func(gtx C) D {
						border := widget.Border{Color: pg.Theme.Color.Gray4, CornerRadius: values.MarginPadding10, Width: values.MarginPadding2}
						wrapper := pg.Theme.Card()
						wrapper.Color = pg.Theme.Color.Gray4
						return border.Layout(gtx, func(gtx C) D {
							return wrapper.Layout(gtx, func(gtx C) D {
								return layout.UniformInset(values.MarginPadding10).Layout(gtx, func(gtx C) D {
									return layout.Flex{}.Layout(gtx,
										layout.Flexed(0.9, pg.Theme.Body1(host).Layout),
										layout.Flexed(0.1, func(gtx C) D {
											return layout.E.Layout(gtx, func(gtx C) D {
												if pg.copyRedirectURL.Clicked() {
													clipboard.WriteOp{Text: host}.Add(gtx.Ops)
													pg.Toast.Notify(values.String(values.StrCopied))
												}
												return pg.copyRedirectURL.Layout(gtx, pg.Theme.Icons.CopyIcon.Layout24dp)
											})
										}),
									)
								})
							})
						})
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{
							Top:  values.MarginPaddingMinus10,
							Left: values.MarginPadding10,
						}.Layout(gtx, func(gtx C) D {
							label := pg.Theme.Body2(values.String(values.StrWebURL))
							label.Color = pg.Theme.Color.GrayText2
							return label.Layout(gtx)
						})
					}),
				)
			}).
			PositiveButton(values.String(values.StrGotIt), modal.DefaultClickFunc())
		pg.ParentWindow().ShowModal(info)
	}

	if pg.syncCompleted {
		time.AfterFunc(time.Second*1, func() {
			pg.syncCompleted = false
			pg.ParentWindow().Reload()
		})
	}

	pg.searchEditor.EditorIconButtonEvent = func() {
		//TODO: consensus search functionality
	}
}

func (pg *ConsensusPage) FetchAgendas() {
	newestFirst := pg.orderDropDown.SelectedIndex() == 0
	selectedWallet := pg.wallets[pg.walletDropDown.SelectedIndex()]

	pg.isSyncing = true

	// Fetch (or re-fetch) agendas in background as this makes
	// a network call. Refresh the window once the call completes.
	go func() {
		pg.consensusItems = components.LoadAgendas(pg.Load, selectedWallet, newestFirst)
		pg.isSyncing = false
		pg.syncCompleted = true
		pg.ParentWindow().Reload()
	}()

	// Refresh the window now to signify that the syncing
	// has started with pg.isSyncing set to true above.
	pg.ParentWindow().Reload()
}

func (pg *ConsensusPage) Layout(gtx C) D {
	if pg.Load.GetCurrentAppWidth() <= gtx.Dp(values.StartMobileView) {
		return pg.layoutMobile(gtx)
	}
	return pg.layoutDesktop(gtx)
}

func (pg *ConsensusPage) layoutDesktop(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(pg.Theme.Label(values.TextSize20, values.String(values.StrConsensusChange)).Layout), // Do we really need to display the title? nav is proposals already
						layout.Rigid(pg.infoButton.Layout),
					)
				}),
				layout.Flexed(1, func(gtx C) D {
					return layout.E.Layout(gtx, pg.layoutRedirectVoting)
				}),
			)
		}),
		layout.Flexed(1, func(gtx C) D {
			return layout.Inset{Top: values.MarginPadding10}.Layout(gtx, func(gtx C) D {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx C) D {
						return layout.Inset{
							Top: values.MarginPadding60,
						}.Layout(gtx, pg.layoutContent)
					}),
					// layout.Expanded(func(gtx C) D {
					// 	gtx.Constraints.Max.X = gtx.Dp(values.MarginPadding150)
					// 	gtx.Constraints.Min.X = gtx.Constraints.Max.X

					//TODO: temp removal till after V1
					// card := pg.Theme.Card()
					// card.Radius = cryptomaterial.Radius(8)
					// return card.Layout(gtx, func(gtx C) D {
					// 	return layout.Inset{
					// 		Left:   values.MarginPadding10,
					// 		Right:  values.MarginPadding10,
					// 		Top:    values.MarginPadding2,
					// 		Bottom: values.MarginPadding2,
					// 	}.Layout(gtx, pg.searchEditor.Layout)
					// })
					// }),
					layout.Expanded(func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.E.Layout(gtx, func(gtx C) D {
							card := pg.Theme.Card()
							card.Radius = cryptomaterial.Radius(8)
							return card.Layout(gtx, func(gtx C) D {
								return layout.UniformInset(values.MarginPadding8).Layout(gtx, func(gtx C) D {
									return pg.layoutSyncSection(gtx)
								})
							})
						})
					}),
					layout.Expanded(func(gtx C) D {
						return pg.orderDropDown.Layout(gtx, 45, true)
					}),
					layout.Expanded(func(gtx C) D {
						return pg.walletDropDown.Layout(gtx, pg.orderDropDown.Width+41, true)
					}),
				)
			})
		}),
	)
}

func (pg *ConsensusPage) layoutMobile(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(pg.Theme.Label(values.TextSize20, values.String(values.StrConsensusChange)).Layout), // Do we really need to display the title? nav is proposals already
						layout.Rigid(pg.infoButton.Layout),
					)
				}),
				layout.Flexed(1, func(gtx C) D {
					return layout.E.Layout(gtx, func(gtx C) D {
						return layout.Inset{Right: values.MarginPadding10, Top: values.MarginPadding5}.Layout(gtx, pg.layoutRedirectVoting)
					})
				}),
			)
		}),
		layout.Flexed(1, func(gtx C) D {
			return layout.Inset{Top: values.MarginPadding10}.Layout(gtx, func(gtx C) D {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx C) D {
						return layout.Inset{
							Top: values.MarginPadding60,
						}.Layout(gtx, pg.layoutContent)
					}),
					// layout.Expanded(func(gtx C) D {
					// 	gtx.Constraints.Max.X = gtx.Dp(values.MarginPadding150)
					// 	gtx.Constraints.Min.X = gtx.Constraints.Max.X

					//TODO: temp removal till after V1
					// card := pg.Theme.Card()
					// card.Radius = cryptomaterial.Radius(8)
					// return card.Layout(gtx, func(gtx C) D {
					// 	return layout.Inset{
					// 		Left:   values.MarginPadding10,
					// 		Right:  values.MarginPadding10,
					// 		Top:    values.MarginPadding2,
					// 		Bottom: values.MarginPadding2,
					// 	}.Layout(gtx, pg.searchEditor.Layout)
					// })
					// }),
					layout.Expanded(func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.E.Layout(gtx, func(gtx C) D {
							card := pg.Theme.Card()
							card.Radius = cryptomaterial.Radius(8)
							return layout.Inset{Right: values.MarginPadding10}.Layout(gtx, func(gtx C) D {
								return card.Layout(gtx, func(gtx C) D {
									return layout.UniformInset(values.MarginPadding8).Layout(gtx, func(gtx C) D {
										return pg.layoutSyncSection(gtx)
									})
								})
							})
						})
					}),
					layout.Expanded(func(gtx C) D {
						return pg.orderDropDown.Layout(gtx, 55, true)
					}),
					layout.Expanded(func(gtx C) D {
						return pg.walletDropDown.Layout(gtx, pg.orderDropDown.Width+51, true)
					}),
				)
			})
		}),
	)
}

func (pg *ConsensusPage) lineSeparator(inset layout.Inset) layout.Widget {
	return func(gtx C) D {
		return inset.Layout(gtx, pg.Theme.Separator().Layout)
	}
}

func (pg *ConsensusPage) layoutRedirectVoting(gtx C) D {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return pg.viewVotingDashboard.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return layout.Inset{
							Right: values.MarginPadding10,
						}.Layout(gtx, pg.redirectIcon.Layout16dp)
					}),
					layout.Rigid(func(gtx C) D {
						return layout.Inset{
							Top: values.MarginPaddingMinus2,
						}.Layout(gtx, pg.Theme.Label(values.TextSize16, values.String(values.StrVotingDashboard)).Layout)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx C) D {
			var text string
			if pg.isSyncing {
				text = values.String(values.StrSyncingState)
			} else if pg.syncCompleted {
				text = values.String(values.StrUpdated)
			}

			lastUpdatedInfo := pg.Theme.Label(values.TextSize10, text)
			lastUpdatedInfo.Color = pg.Theme.Color.GrayText2
			if pg.syncCompleted {
				lastUpdatedInfo.Color = pg.Theme.Color.Success
			}

			return layout.E.Layout(gtx, func(gtx C) D {
				return layout.Inset{Top: values.MarginPadding2}.Layout(gtx, lastUpdatedInfo.Layout)
			})
		}),
	)
}

func (pg *ConsensusPage) layoutContent(gtx C) D {
	if len(pg.consensusItems) == 0 {
		return components.LayoutNoAgendasFound(gtx, pg.Load, pg.isSyncing)
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx C) D {
			list := layout.List{Axis: layout.Vertical}
			return pg.Theme.List(pg.listContainer).Layout(gtx, 1, func(gtx C, i int) D {
				return layout.Inset{Right: values.MarginPadding2}.Layout(gtx, func(gtx C) D {
					return list.Layout(gtx, len(pg.consensusItems), func(gtx C, i int) D {
						return cryptomaterial.LinearLayout{
							Orientation: layout.Vertical,
							Width:       cryptomaterial.MatchParent,
							Height:      cryptomaterial.WrapContent,
							Background:  pg.Theme.Color.Surface,
							Direction:   layout.W,
							Border:      cryptomaterial.Border{Radius: cryptomaterial.Radius(14)},
							Padding:     layout.UniformInset(values.MarginPadding15),
							Margin:      layout.Inset{Bottom: values.MarginPadding4, Top: values.MarginPadding4}}.
							Layout2(gtx, func(gtx C) D {
								return components.AgendaItemWidget(gtx, pg.Load, pg.consensusItems[i])
							})
					})
				})
			})
		}),
	)
}

func (pg *ConsensusPage) layoutSyncSection(gtx C) D {
	if pg.isSyncing {
		return pg.layoutIsSyncingSection(gtx)
	} else if pg.syncCompleted {
		updatedIcon := cryptomaterial.NewIcon(pg.Theme.Icons.NavigationCheck)
		updatedIcon.Color = pg.Theme.Color.Success
		return updatedIcon.Layout(gtx, values.MarginPadding20)
	}
	return pg.layoutStartSyncSection(gtx)
}

func (pg *ConsensusPage) layoutIsSyncingSection(gtx C) D {
	th := material.NewTheme(gofont.Collection())
	gtx.Constraints.Max.X = gtx.Dp(values.MarginPadding24)
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	loader := material.Loader(th)
	loader.Color = pg.Theme.Color.Gray1
	return loader.Layout(gtx)
}

func (pg *ConsensusPage) layoutStartSyncSection(gtx C) D {
	// TODO: use cryptomaterial clickable
	return material.Clickable(gtx, pg.syncButton, pg.Theme.Icons.Restore.Layout24dp)
}
